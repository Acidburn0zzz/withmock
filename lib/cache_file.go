// Copyright 2013 Julian Phillips.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lib

import (
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"go/ast"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

const CacheData = "_DATA_"

func init() {
	gob.Register(map[string]bool{})
	gob.Register(&mockFileInfo{})
	registerAstTypes()
}

type CacheFile struct {
	key *CacheFileKey
	f *os.File
	tmpName string
	written bool
	changed bool
	h hash.Hash
	cache *Cache
	hash string
	data map[string]interface{}
}

func (c *Cache) loadFile(key *CacheFileKey) (*CacheFile, error) {
	cf := &CacheFile{
		key: key,
		written: false,
		changed: false,
		h: NewCacheHash(),
		cache: c,
		hash: "",
		data: nil,
	}

	path := filepath.Join(c.root, "metadata", key.Hash())

	f, err := os.Open(path)

	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := gob.NewDecoder(f)

	if err := dec.Decode(&cf.data); err != nil {
		return nil, Cerr{"gob.Decode", err}
	}

	cf.hash = cf.data[CacheData].(string)

	return cf, nil
}

func (c *Cache) GetFile(key *CacheFileKey) (*CacheFile, error) {
	cf, err := c.loadFile(key)
	if err == nil {
		return cf, nil
	}

	if !os.IsNotExist(err) {
		return nil, Cerr{"loadFile", err}
	}

	return &CacheFile{
		key: key,
		written: false,
		changed: false,
		h: NewCacheHash(),
		cache: c,
		hash: "",
		data: make(map[string]interface{}),
	}, nil
}

func (f *CacheFile) open() error {
	dir := filepath.Join(f.cache.root, "files")

	if err := os.MkdirAll(dir, 0700); err != nil {
		return Cerr{"os.MkdirAll", err}
	}

	w, err := ioutil.TempFile(dir, "withmock-data-")
	if err != nil {
		return Cerr{"TempFile", err}
	}

	f.f = w
	f.tmpName = w.Name()

	return nil
}

func (f *CacheFile) dump() error {
	if f.f == nil {
		return nil
	}

	if err := f.f.Close(); err != nil {
		return Cerr{"os.File.Close", err}
	}
	f.f = nil

	if err := os.Remove(f.tmpName); err != nil {
		return Cerr{"os.Remove", err}
	}

	return nil
}

func (f *CacheFile) Write(p []byte) (int, error) {
	if f.f == nil {
		if err := f.open(); err != nil {
			return 0, Cerr{"cf.open", err}
		}
	}

	f.written = true
	return io.MultiWriter(f.f, f.h).Write(p)
}

func (f *CacheFile) WriteFunc(w func(string) error) error {
	f.written = true

	// Currently creating a tempfile is the only way to get a filename ...

	if err := f.open(); err != nil {
		return Cerr{"cf.open", err}
	}

	// Get rid of f.f - w is going to make the file directly

	if err := f.dump(); err != nil {
		return Cerr{"f.dump", err}
	}

	// Call w to write the file content

	if err := w(f.tmpName); err != nil {
		return Cerr{"w", err}
	}

	// And now we need to read it into the hash

	i, err := os.Open(f.tmpName)
	if os.IsNotExist(err) {
		// w didn't actually create a file - so reset the written flag, and we
		// are done.
		f.written = false
		return nil
	}
	if err != nil {
		return Cerr{"os.Open", err}
	}
	defer i.Close()

	if _, err := io.Copy(f.h, i); err != nil {
		return Cerr{"io.Copy", err}
	}

	// done

	return nil
}

func (f *CacheFile) Hash() string {
	return f.hash
}

func (f *CacheFile) Close() error {
	if !f.written {
		if err := f.dump(); err != nil {
			return Cerr{"f.dump", err}
		}

		return nil
	}

	// if hash has been set, then we are already closed
	if f.hash != "" {
		if err := f.dump(); err != nil {
			return Cerr{"f.dump", err}
		}

		return nil
	}

	f.changed = true

	if f.f != nil {
		if err := f.f.Close(); err != nil {
			return Cerr{"os.File.Close", err}
		}
		f.f = nil
	}

	f.hash = hex.EncodeToString(f.h.Sum(nil))

	name := filepath.Join(f.cache.root, "files", f.hash)

	if err := os.Rename(f.tmpName, name); err != nil {
		return Cerr{"os.Rename", err}
	}

	if err := os.Chmod(name, 0400); err != nil {
		return Cerr{"os.Chmod", err}
	}

	f.data[CacheData] = f.hash

	return nil
}

func (f *CacheFile) Install(path string) error {
	if err := f.Close(); err != nil {
		return Cerr{"f.Close", err}
	}

	if f.written || f.HasData() {
		// Get the hash from data - as we could be installing either a new file,
		// or one entirely loaded from the cache ...
		hash, found := f.data[CacheData]
		if !found {
			return fmt.Errorf("Failed to get hash")
		}

		dir := filepath.Dir(path)

		if err := os.MkdirAll(dir, 0700); err != nil {
			return Cerr{"os.MkdirAll", err}
		}

		name := filepath.Join(f.cache.root, "files", hash.(string))

		if err := os.Link(name, path); err != nil {
			if err := os.Symlink(name, path); err != nil {
				return Cerr{"os.Symlink", err}
			}
		}
	}

	if err := f.Save(); err != nil {
		return Cerr{"f.Save", err}
	}

	return nil
}

func (f *CacheFile) Save() error {
	if !f.changed {
		return nil
	}

	dir := filepath.Join(f.cache.root, "metadata")

	if err := os.MkdirAll(dir, 0700); err != nil {
		return Cerr{"os.MkdirAll", err}
	}

	w, err := ioutil.TempFile(dir, "withmock-meta-")
	if err != nil {
		return Cerr{"TempFile", err}
	}
	defer w.Close()

	enc := gob.NewEncoder(w)

	if err := enc.Encode(f.data); err != nil {
		return Cerr{"gob.Encode", err}
	}

	path := filepath.Join(f.cache.root, "metadata", f.key.Hash())

	w.Close()
	if err := os.Rename(w.Name(), path); err != nil {
		return Cerr{"os.Rename", err}
	}

	return nil
}

func (f *CacheFile) HasData() bool {
	return f.Has(CacheData)
}

func (f *CacheFile) Has(name ...string) bool {
	for _, n := range name {
		_, found := f.data[n]
		if !found {
			return false
		}
	}

	return true
}

func (f *CacheFile) Store(name string, data interface{}) {
	if name[0] == '_' {
		panic("Attempt to set private data member: " + name)
	}

	f.data[name] = data
}

func (f *CacheFile) Get(name string) interface{} {
	return f.data[name]
}

func (f *CacheFile) Lookup(name string) (interface{}, bool) {
	value, found := f.data[name]
	return value, found
}

func registerAstTypes() {
	gob.Register(&ast.ArrayType{})
	gob.Register(&ast.AssignStmt{})
	gob.Register(&ast.BadDecl{})
	gob.Register(&ast.BadExpr{})
	gob.Register(&ast.BadStmt{})
	gob.Register(&ast.BasicLit{})
	gob.Register(&ast.BinaryExpr{})
	gob.Register(&ast.BlockStmt{})
	gob.Register(&ast.BranchStmt{})
	gob.Register(&ast.CallExpr{})
	gob.Register(&ast.CaseClause{})
	gob.Register(&ast.ChanType{})
	gob.Register(&ast.CommClause{})
	gob.Register(&ast.Comment{})
	gob.Register(&ast.CommentGroup{})
	gob.Register(&ast.CompositeLit{})
	gob.Register(&ast.DeclStmt{})
	gob.Register(&ast.DeferStmt{})
	gob.Register(&ast.Ellipsis{})
	gob.Register(&ast.EmptyStmt{})
	gob.Register(&ast.ExprStmt{})
	gob.Register(&ast.Field{})
	gob.Register(&ast.FieldList{})
	gob.Register(&ast.File{})
	gob.Register(&ast.ForStmt{})
	gob.Register(&ast.FuncDecl{})
	gob.Register(&ast.FuncLit{})
	gob.Register(&ast.FuncType{})
	gob.Register(&ast.GenDecl{})
	gob.Register(&ast.GoStmt{})
	gob.Register(&ast.Ident{})
	gob.Register(&ast.IfStmt{})
	gob.Register(&ast.ImportSpec{})
	gob.Register(&ast.IncDecStmt{})
	gob.Register(&ast.IndexExpr{})
	gob.Register(&ast.InterfaceType{})
	gob.Register(&ast.KeyValueExpr{})
	gob.Register(&ast.LabeledStmt{})
	gob.Register(&ast.MapType{})
	gob.Register(&ast.Object{})
	gob.Register(&ast.Package{})
	gob.Register(&ast.ParenExpr{})
	gob.Register(&ast.RangeStmt{})
	gob.Register(&ast.ReturnStmt{})
	gob.Register(&ast.Scope{})
	gob.Register(&ast.SelectStmt{})
	gob.Register(&ast.SelectorExpr{})
	gob.Register(&ast.SendStmt{})
	gob.Register(&ast.SliceExpr{})
	gob.Register(&ast.StarExpr{})
	gob.Register(&ast.StructType{})
	gob.Register(&ast.SwitchStmt{})
	gob.Register(&ast.TypeAssertExpr{})
	gob.Register(&ast.TypeSpec{})
	gob.Register(&ast.TypeSwitchStmt{})
	gob.Register(&ast.UnaryExpr{})
	gob.Register(&ast.ValueSpec{})
}
