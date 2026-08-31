// Package function is the Google Cloud Functions Gen 2 entry package for the
// full gospace worker. The Functions buildpack owns main(); this package only
// exposes the exact bare HTTP function type it expects.
package function

import (
	"net/http"

	"github.com/dash-xd/gospace/internal/gospace"
)

var Main func(http.ResponseWriter, *http.Request) = gospace.Main
