// Copyright 2013 Julian Phillips.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lib

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"time"
	"io"
	"log"
)

type cacheFileDetails struct {
	Src string `json:"src"`
	Size int64 `json:"size"`
	Mode os.FileMode `json:"mode"`
	ModTime time.Time `json:"mod_time"`
	Hash string `json:"hash"`
}

func (c *Cache) getDetails(path string) (cacheFileDetails, error) {
	// TODO: need to include file size, mode, hash etc in key ...

	st, err := os.Stat(path)
	if err != nil {
		return cacheFileDetails{}, Cerr{"os.Stat", err}
	}

	f, err := os.Open(path)
	if err != nil {
		return cacheFileDetails{}, Cerr{"os.Open", err}
	}
	defer f.Close()

	h := NewCacheHash()

	log.Printf("START: calcHash")
	if _, err := io.Copy(h, f); err != nil {
		return cacheFileDetails{}, Cerr{"io.Copy", err}
	}
	hash := hex.EncodeToString(h.Sum(nil))
	log.Printf("END: calcHash")

	return cacheFileDetails{
		Src: path,
		Size: st.Size(),
		Mode: st.Mode(),
		ModTime: st.ModTime(),
		Hash: hash,
	}, nil
}

type CacheFileKey struct {
	Op string `json:"op"`
	Files []cacheFileDetails `json:"files"`
	hash string
}

func (c *Cache) NewCacheFileKey(op string, srcs ...string) (*CacheFileKey, error) {
	var err error

	files := make([]cacheFileDetails, len(srcs))
	for i, src := range srcs {
		log.Printf("START: getDetails")
		files[i], err = c.getDetails(src)
		log.Printf("END: getDetails")
		if err != nil {
			return nil, Cerr{"c.getDetails("+src+")", err}
		}
	}

	return &CacheFileKey{
		Op: op,
		Files: files,
	}, nil
}

func (k *CacheFileKey) Hash() string {
	if k.hash == "" {
		k.calcHash()
	}

	return k.hash
}

func (k *CacheFileKey) calcHash() {
	h := NewCacheHash()

	enc := json.NewEncoder(h)

	if err := enc.Encode(k); err != nil {
		panic("Failed to JSON encode cacheFileKey instance: " + err.Error())
	}

	k.hash = hex.EncodeToString(h.Sum(nil))
}
