package frontend

import (
	"embed"
	"path"
	"strings"
)

//go:embed all:dist static
var files embed.FS

func DefaultIndex() string {
	return mustRead("static/default-index.html")
}

func LoginHTML() string {
	return mustRead("static/login.html")
}

func ReadClientJS() ([]byte, error) {
	return files.ReadFile("static/flink.js")
}

func ReadDist(name string) ([]byte, string, error) {
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	b, err := files.ReadFile("dist/" + name)
	return b, name, err
}

func mustRead(name string) string {
	b, err := files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return string(b)
}
