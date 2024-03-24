package firefox_container

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pierrec/lz4"
	"github.com/zellyn/kooky/firefox"
)

func JoinCookiesToString(cookies []http.Cookie) string {
	var builder strings.Builder
	for i, cookie := range cookies {
		builder.WriteString(cookie.Name)
		builder.WriteString(": ")
		builder.WriteString(cookie.Value)
		if i < len(cookies)-1 {
			builder.WriteString("; ")
		}
	}
	return builder.String()
}

type CookieReader interface {
	Read(path string) ([]http.Cookie, error)
}

// Jsonlz4CookieReader reads the Data/profile/sessionstore-backups/recovery.jsonlz4 file
type Jsonlz4CookieReader struct {
	LeadingCompressedBytesIgnoredAmount   int
	LeadingDecompressedBytesIgnoredAmount int

	CompressionFactorRangeMin  int
	CompressionFactorRangeMax  int
	CompressionFactorRangeStep int
}

func NewJsonlz4CookieReader() Jsonlz4CookieReader {
	return Jsonlz4CookieReader{
		LeadingCompressedBytesIgnoredAmount:   2,
		LeadingDecompressedBytesIgnoredAmount: 21,

		CompressionFactorRangeMin:  5,
		CompressionFactorRangeMax:  30,
		CompressionFactorRangeStep: 5,
	}
}

func (r Jsonlz4CookieReader) Read(path string) ([]http.Cookie, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error while opening %s: %v", path, err)
	}
	reader := bufio.NewReader(file)
	for i := 0; i < r.LeadingCompressedBytesIgnoredAmount; i++ {
		if _, err := reader.ReadByte(); err != nil {
			return nil, fmt.Errorf("error while skipping leading compressed bytes: %v", err)
		}
	}

	compressedBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error while reading compressed bytes: %v", err)
	}

	var decompressedBytes []byte
	var wErr error
	for compressionFactor := r.CompressionFactorRangeMin; compressionFactor <= r.CompressionFactorRangeMax; compressionFactor += r.CompressionFactorRangeStep {
		decompressedBytes = make([]byte, len(compressedBytes)*compressionFactor)
		decompressedSize, err := lz4.UncompressBlock(compressedBytes, decompressedBytes)
		if err != nil {
			if errors.Is(lz4.ErrInvalidSourceShortBuffer, err) {
				wErr = err
				continue
			}
			return nil, fmt.Errorf("error while trying to decompress bytes with factor %d: %v", compressionFactor, err)
		}
		decompressedBytes = decompressedBytes[r.LeadingDecompressedBytesIgnoredAmount:decompressedSize]
		wErr = nil
		break
	}
	if wErr != nil {
		return nil, fmt.Errorf("error while trying to decompress bytes: %v", wErr)
	}

	type FileContents struct {
		Cookies []struct {
			Expiry    int    `json:"expiry"`
			Host      string `json:"host"`
			HttpOnly  bool   `json:"httponly"`
			Name      string `json:"name"`
			Path      string `json:"path"`
			SameSite  int    `json:"sameSite"`
			SchemaMap int    `json:"schemeMap"`
			Secure    bool   `json:"secure"`
			Value     string `json:"value"`
		} `json:"cookies"`
	}

	var contents FileContents
	if err := json.Unmarshal(decompressedBytes, &contents); err != nil {
		return nil, fmt.Errorf("error while unmarshaling the decompressed bytes: %v", err)
	}
	cookies := make([]http.Cookie, len(contents.Cookies))
	for _, cookie := range contents.Cookies {
		var httpSameSite http.SameSite
		sameSite := cookie.SameSite
		if sameSite == 1 {
			httpSameSite = http.SameSiteDefaultMode
		}
		cookies = append(cookies, http.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Host,
			HttpOnly: cookie.HttpOnly,
			SameSite: httpSameSite,
			Secure:   cookie.Secure,
			Expires:  time.Unix(0, int64(cookie.Expiry)*int64(time.Millisecond)),
		})
	}
	return cookies, nil
}

// CookiesSqliteCookieReader reads the Data/profile/cookies.sqlite file
type CookiesSqliteCookieReader struct{}

func (CookiesSqliteCookieReader) Read(path string) ([]http.Cookie, error) {
	kookies, err := firefox.ReadCookies(path)
	if err != nil {
		return nil, fmt.Errorf("error while reading cookies from %s: %v", path, err)
	}
	cookies := make([]http.Cookie, len(kookies))
	for i, kookie := range kookies {
		cookies[i] = kookie.HTTPCookie()
	}
	return cookies, nil
}
