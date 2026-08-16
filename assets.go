package oida

import (
	"embed"
	"io/fs"
	"strings"
	"sync"

	"github.com/a-h/templ"
)

// publicFS holds the front end assets. The whole tree is embedded, so dropping
// a file into public/assets serves it without touching this file.
//
//go:embed all:public
var publicFS embed.FS

// assetPrefix is the route the asset tree is served under, relative to the
// mount path.
const assetPrefix = "/assets/"

// assets returns the embedded public tree rooted at public/, so paths line up
// with the URLs they are served at.
var assets = sync.OnceValue(func() fs.FS {
	sub, err := fs.Sub(publicFS, "public")
	if err != nil {
		// Unreachable: the directory is embedded at compile time.
		return publicFS
	}
	return sub
})

// asset reads one embedded asset, returning an empty string when it is absent.
func asset(name string) string {
	data, err := fs.ReadFile(assets(), name)
	if err != nil {
		return ""
	}
	return string(data)
}

// styleSheet is the front end stylesheet.
var styleSheet = sync.OnceValue(func() string {
	return asset("assets/oida.css")
})

// assetMarker is the placeholder the stylesheet uses for its own asset URLs.
// The sheet is served from {Path}/assets/ and also inlined into a document at
// {Path}, so a relative URL cannot be right in both places.
const assetMarker = "asset:"

// styleSheetFor returns the stylesheet with its asset URLs pointed at base.
func styleSheetFor(base string) string {
	return strings.ReplaceAll(styleSheet(), assetMarker, base)
}

// StyleSheet returns the embedded stylesheet of the debug front end, with its
// asset URLs resolved against base, which is the mount path plus /assets/.
func StyleSheet(base string) string {
	return styleSheetFor(base)
}

// LiveScript returns the embedded live view script of the debug front end. It
// subscribes to the event stream and swaps in the section the server renders.
func LiveScript() string {
	return asset("assets/oida.js")
}

// styleElement returns the stylesheet wrapped in a style element. It is inlined
// into the document head so the page renders standalone when the asset route is
// unreachable. The content is embedded at compile time, never user input.
func styleElement(base string) templ.Component {
	return templ.Raw("<style>" + styleSheetFor(base) + "</style>")
}
