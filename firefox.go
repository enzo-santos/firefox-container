package firefox_container

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

type FirefoxPortable struct {
	Path           string
	ExecutableName string
}

func (f FirefoxPortable) ExecutablePath() string {
	return filepath.Join(f.Path, f.ExecutableName)
}

type FirefoxLoadOptions struct {
	// OpenBrowser decides whether the Firefox Portable executable should be launched so that the user can enter their
	// credential data manually.
	//
	// The executable is launched only if there is no authentication token stored locally or if the existing token is
	// already expired.
	OpenBrowser bool

	// Logger is the logger to be used during the load operation.
	Logger *log.Logger

	// OnStartListening is called when the load operation starts to listen the authentication token file for changes.
	//
	// If there is an authentication token stored locally, and it's not expired, this callback is never called.
	OnStartListening func()
}

func (f FirefoxPortable) Load(extractor TokenExtractor, options FirefoxLoadOptions) (string, error) {
	var logger *log.Logger
	if optionsLogger := options.Logger; optionsLogger == nil {
		logger = log.Default()
	} else {
		logger = optionsLogger
	}

	databasePath := filepath.Join(f.Path, extractor.GetFilePath())

	// If the database file already exists on disk, check if the stored token is valid
	bearerToken, bearerTokenErr := extractor.Parse(databasePath)
	if bearerTokenErr == nil {
		isTokenActive, err := extractor.Validate(bearerToken)
		if err != nil {
			return "", fmt.Errorf("error while checking if the token found is fresh: %v", err)
		}
		if isTokenActive {
			return bearerToken, nil
		}
	}
	// There is no file to read from or the token is expired

	// Check if the user wants to authenticate manually
	if options.OpenBrowser {
		cmd := exec.Command(f.ExecutablePath(), "-new-tab", extractor.GetLoginUrl().String())
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		}
		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("error while launching the browser: %v", err)
		}
		defer func() {
			// Here, errors are ignored because this is an operation implemented only for convenience. That is, it's not
			// an important operation: if the browser is not closed by this operation, the user can close it manually.
			pid := cmd.Process.Pid

			if process, err := os.FindProcess(pid); err == nil {
				if process == nil {
					logger.Printf("os.FindProcess has not found any process with PID %d\n", pid)
				} else {
					if err := process.Kill(); err == nil {
						logger.Printf("process.Kill has been called successfully on PID %d\n", pid)
					} else {
						logger.Printf("process.Kill returned an error for PID %d: %v\n", pid, err)
					}
				}
			} else {
				logger.Printf("os.FindProcess returned an error for PID %d: %v", pid, err)
			}
		}()
	}

	// Listen to the root directory for changes
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return "", fmt.Errorf("error while creating file system watcher: %v", err)
	}
	defer watcher.Close()

	var wg = &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		if onStartListening := options.OnStartListening; onStartListening != nil {
			onStartListening()
		}
		defer wg.Done()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					bearerToken = ""
					bearerTokenErr = nil
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) && filepath.Base(event.Name) == filepath.Base(databasePath) {
					if result, err := extractor.Parse(event.Name); err != nil {
						bearerToken = ""
						bearerTokenErr = err
						return
					} else {
						// Even if it finds a bearer token, the loop will only stop if it finds a valid one
						isTokenFresh, err := extractor.Validate(result)
						if err != nil {
							bearerToken = ""
							bearerTokenErr = err
							return
						}
						if isTokenFresh {
							bearerToken = result
							bearerTokenErr = nil
							return
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					bearerToken = ""
					bearerTokenErr = nil
				} else {
					bearerToken = ""
					bearerTokenErr = err
				}
				return
			}
		}
	}()

	databaseRootPath := filepath.Dir(databasePath)
	if err := watcher.Add(databaseRootPath); err != nil {
		return "", fmt.Errorf("error while listening to %s: %v", databaseRootPath, err)
	}
	wg.Wait()

	// At this point of execution, `bearerToken` and `bearerTokenErr` are set
	if bearerTokenErr != nil {
		return "", fmt.Errorf("error while trying to retrieve the bearer token: %v", bearerTokenErr)
	}
	// At this point of execution, `bearerToken` contains a valid bearer token
	return bearerToken, nil
}
