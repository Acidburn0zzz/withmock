// Copyright 2013 Julian Phillips.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lib

import (
	"fmt"
	"path/filepath"
	"strings"
	"os"
	"log"
)

var (
	goRoot, goPath string
)

func init() {
	var err error

	goRoot, err = GetOutput("go", "env", "GOROOT")
	if err != nil {
		panic("Unable to get GOROOT: " + err.Error())
	}

	goPath, err = GetOutput("go", "env", "GOPATH")
	if err != nil {
		panic("Unable to get GOPATH: " + err.Error())
	}
}

func find(impPath string) string {
	path := filepath.Join(goRoot, "src", "pkg", impPath)
	log.Printf("stat: %s", path)
	if _, err := os.Stat(path); err == nil {
		return path
	}

	for _, prefix := range filepath.SplitList(goPath) {
		path := filepath.Join(prefix, "src", impPath)
		log.Printf("stat: %s", path)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func LookupImportPath(impPath string) (string, error) {
	if strings.HasPrefix(impPath, "_/") {
		// special case if impPath is outside of GOPATH
		return impPath[1:], nil
	}

	path := find(impPath)
	if path == "" {
		return "", fmt.Errorf("Unable to find package: %s", impPath)
	}

	path, err := filepath.Abs(path)
	if err != nil {
		return "", Cerr{"filepath.Abs", err}
	}

	return path, nil
}
