package api

import (
	"net/url"
	"strings"
	"testing"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/modules"
)

// TestDefaultPath ensures defaultPath functions correctly.
func TestDefaultPath(t *testing.T) {
	tests := []struct {
		name               string
		queryForm          url.Values
		subfiles           modules.SkyfileSubfiles
		defaultPath        string
		disableDefaultPath bool
		err                error
	}{
		{
			name:               "single file not multipart nil",
			queryForm:          url.Values{},
			subfiles:           nil,
			defaultPath:        "",
			disableDefaultPath: false,
			err:                nil,
		},
		{
			name:               "single file not multipart empty",
			queryForm:          url.Values{modules.SkyfileDisableDefaultPathParamName: []string{"true"}},
			subfiles:           nil,
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},
		{
			name:               "single file not multipart set",
			queryForm:          url.Values{modules.SkyfileDefaultPathParamName: []string{"about.html"}},
			subfiles:           nil,
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},

		{
			name:               "single file multipart nil",
			queryForm:          url.Values{},
			subfiles:           modules.SkyfileSubfiles{"about.html": modules.PubfileSubfileMetadata{}},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                nil,
		},
		{
			name:               "single file multipart empty",
			queryForm:          url.Values{modules.SkyfileDisableDefaultPathParamName: []string{"true"}},
			subfiles:           modules.SkyfileSubfiles{"about.html": modules.PubfileSubfileMetadata{}},
			defaultPath:        "",
			disableDefaultPath: true,
			err:                nil,
		},
		{
			name:               "single file multipart set to only",
			queryForm:          url.Values{modules.SkyfileDefaultPathParamName: []string{"about.html"}},
			subfiles:           modules.SkyfileSubfiles{"about.html": modules.PubfileSubfileMetadata{}},
			defaultPath:        "/about.html",
			disableDefaultPath: false,
			err:                nil,
		},
		{
			name:               "single file multipart set to nonexistent",
			queryForm:          url.Values{modules.SkyfileDefaultPathParamName: []string{"nonexistent.html"}},
			subfiles:           modules.SkyfileSubfiles{"about.html": modules.PubfileSubfileMetadata{}},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},
		{
			name:               "single file multipart set to non-html",
			queryForm:          url.Values{modules.SkyfileDefaultPathParamName: []string{"about.js"}},
			subfiles:           modules.SkyfileSubfiles{"about.js": modules.PubfileSubfileMetadata{}},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},
		{
			name: "single file multipart both set",
			queryForm: url.Values{
				modules.SkyfileDefaultPathParamName:        []string{"about.html"},
				modules.SkyfileDisableDefaultPathParamName: []string{"true"},
			},
			subfiles:           modules.SkyfileSubfiles{"about.html": modules.PubfileSubfileMetadata{}},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},
		{
			name:               "single file multipart set to non-root",
			queryForm:          url.Values{modules.SkyfileDefaultPathParamName: []string{"foo/bar/about.html"}},
			subfiles:           modules.SkyfileSubfiles{"foo/bar/about.html": modules.PubfileSubfileMetadata{}},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},

		{
			name:      "multi file nil has index.html",
			queryForm: url.Values{},
			subfiles: modules.SkyfileSubfiles{
				"about.html": modules.PubfileSubfileMetadata{},
				"index.html": modules.PubfileSubfileMetadata{},
			},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                nil,
		},
		{
			name:      "multi file nil no index.html",
			queryForm: url.Values{},
			subfiles: modules.SkyfileSubfiles{
				"about.html": modules.PubfileSubfileMetadata{},
				"hello.html": modules.PubfileSubfileMetadata{},
			},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                nil,
		},
		{
			name:      "multi file set to empty",
			queryForm: url.Values{modules.SkyfileDisableDefaultPathParamName: []string{"true"}},
			subfiles: modules.SkyfileSubfiles{
				"about.html": modules.PubfileSubfileMetadata{},
				"index.html": modules.PubfileSubfileMetadata{},
			},
			defaultPath:        "",
			disableDefaultPath: true,
			err:                nil,
		},
		{
			name:      "multi file set to existing",
			queryForm: url.Values{modules.SkyfileDefaultPathParamName: []string{"about.html"}},
			subfiles: modules.SkyfileSubfiles{
				"about.html": modules.PubfileSubfileMetadata{},
				"index.html": modules.PubfileSubfileMetadata{},
			},
			defaultPath:        "/about.html",
			disableDefaultPath: false,
			err:                nil,
		},
		{
			name:      "multi file set to nonexistent",
			queryForm: url.Values{modules.SkyfileDefaultPathParamName: []string{"nonexistent.html"}},
			subfiles: modules.SkyfileSubfiles{
				"about.html": modules.PubfileSubfileMetadata{},
				"index.html": modules.PubfileSubfileMetadata{},
			},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},
		{
			name:      "multi file set to non-html",
			queryForm: url.Values{modules.SkyfileDefaultPathParamName: []string{"about.js"}},
			subfiles: modules.SkyfileSubfiles{
				"about.js":   modules.PubfileSubfileMetadata{},
				"index.html": modules.PubfileSubfileMetadata{},
			},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},
		{
			name: "multi file both set",
			queryForm: url.Values{
				modules.SkyfileDefaultPathParamName:        []string{"about.html"},
				modules.SkyfileDisableDefaultPathParamName: []string{"true"},
			},
			subfiles: modules.SkyfileSubfiles{
				"about.html": modules.PubfileSubfileMetadata{},
				"index.html": modules.PubfileSubfileMetadata{},
			},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},
		{
			name:      "multi file set to non-root",
			queryForm: url.Values{modules.SkyfileDefaultPathParamName: []string{"foo/bar/about.html"}},
			subfiles: modules.SkyfileSubfiles{
				"foo/bar/about.html": modules.PubfileSubfileMetadata{},
				"foo/bar/baz.html":   modules.PubfileSubfileMetadata{},
			},
			defaultPath:        "",
			disableDefaultPath: false,
			err:                ErrInvalidDefaultPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp, ddp, err := defaultPath(tt.queryForm, tt.subfiles)
			if (err != nil || tt.err != nil) && !errors.Contains(err, tt.err) {
				t.Fatalf("Expected error %v, got %v\n", tt.err, err)
			}
			if dp != tt.defaultPath {
				t.Fatalf("Expected defaultPath '%v', got '%v'\n", tt.defaultPath, dp)
			}
			if ddp != tt.disableDefaultPath {
				t.Fatalf("Expected disableDefaultPath '%v', got '%v'\n", tt.disableDefaultPath, ddp)
			}
		})
	}
}

// TestSplitSkylinkString is a table test for the splitSkylinkString function.
func TestSplitSkylinkString(t *testing.T) {
	tests := []struct {
		name                 string
		strToParse           string
		publink              string
		skylinkStringNoQuery string
		path                 string
		errMsg               string
	}{
		{
			name:                 "no path",
			strToParse:           "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			publink:              "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			skylinkStringNoQuery: "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			path:                 "/",
			errMsg:               "",
		},
		{
			name:                 "no path with query",
			strToParse:           "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w?foo=bar",
			publink:              "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			skylinkStringNoQuery: "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			path:                 "/",
			errMsg:               "",
		},
		{
			name:                 "with path to file",
			strToParse:           "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar.baz",
			publink:              "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			skylinkStringNoQuery: "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar.baz",
			path:                 "/foo/bar.baz",
			errMsg:               "",
		},
		{
			name:                 "with path to dir with trailing slash",
			strToParse:           "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar/",
			publink:              "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			skylinkStringNoQuery: "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar/",
			path:                 "/foo/bar/",
			errMsg:               "",
		},
		{
			name:                 "with path to dir without trailing slash",
			strToParse:           "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar",
			publink:              "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			skylinkStringNoQuery: "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar",
			path:                 "/foo/bar",
			errMsg:               "",
		},
		{
			name:                 "with path to file with query",
			strToParse:           "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar.baz?foobar=nope",
			publink:              "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			skylinkStringNoQuery: "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar.baz",
			path:                 "/foo/bar.baz",
			errMsg:               "",
		},
		{
			name:                 "with path to dir with query with trailing slash",
			strToParse:           "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar/?foobar=nope",
			publink:              "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			skylinkStringNoQuery: "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar/",
			path:                 "/foo/bar/",
			errMsg:               "",
		},
		{
			name:                 "with path to dir with query without trailing slash",
			strToParse:           "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar?foobar=nope",
			publink:              "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w",
			skylinkStringNoQuery: "IAC6CkhNYuWZqMVr1gob1B6tPg4MrBGRzTaDvAIAeu9A9w/foo/bar",
			path:                 "/foo/bar",
			errMsg:               "",
		},
		{
			name:                 "invalid publink",
			strToParse:           "invalid_publink/foo/bar?foobar=nope",
			publink:              "",
			skylinkStringNoQuery: "",
			path:                 "",
			errMsg:               modules.ErrPublinkIncorrectSize.Error(),
		},
		{
			name:                 "empty input",
			strToParse:           "",
			publink:              "",
			skylinkStringNoQuery: "",
			path:                 "",
			errMsg:               modules.ErrPublinkIncorrectSize.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publink, skylinkStringNoQuery, path, err := splitSkylinkString(tt.strToParse)
			if (err != nil || tt.errMsg != "") && !strings.Contains(err.Error(), tt.errMsg) {
				t.Fatalf("Expected error '%s', got %v\n", tt.errMsg, err)
			}
			if tt.errMsg != "" {
				return
			}
			if publink.String() != tt.publink {
				t.Fatalf("Expected publink '%v', got '%v'\n", tt.publink, publink)
			}
			if skylinkStringNoQuery != tt.skylinkStringNoQuery {
				t.Fatalf("Expected skylinkStringNoQuery '%v', got '%v'\n", tt.skylinkStringNoQuery, skylinkStringNoQuery)
			}
			if path != tt.path {
				t.Fatalf("Expected path '%v', got '%v'\n", tt.path, path)
			}
		})
	}
}
