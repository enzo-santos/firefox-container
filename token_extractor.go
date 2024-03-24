package firefox_container

import (
	"net/url"
)

type TokenExtractorListenEvent int

const (
	CreateListenEvent = 1 << iota
	WriteListenEvent
)

type TokenExtractor interface {
	// GetListenEvent should return to which directory listening event (create or update) the driver should react to.
	GetListenEvent() TokenExtractorListenEvent

	// GetLoginUrl should return a URL that the user can visit so that they can enter their credential data manually.
	GetLoginUrl() *url.URL

	// GetFilePath should return the path of the file where the authentication token will be extracted.
	//
	// The returned path should be relative to the Firefox Portable root directory.
	GetFilePath() string

	// Parse should, given its path, read a file that has the same format as the file pointed by GetFilePath and return
	// the relevant authentication token contained in it.
	Parse(path string) (string, error)

	// Validate should, given an authentication token, check if it's valid or already expired.
	//
	// A token is considered expired if using it on a request raises 401 Unauthorized or 403 Forbidden, where it used to
	// return a 200-like HTTP code at some point in the past.
	Validate(token string) (bool, error)
}
