// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package easyfs

import (
	"io"
	"io/fs"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Do not import fmt in this package, this is meant to be a low-level package.

// To use: first create an empty in-memory file system
// fs := easyfs.MapFS{}
// Then add files to it:
// fs["foo/test.txt"] = &easyfs.MapFile{Data: []byte("hello")}
// or
// fs.WriteFile("foo/test.txt", []byte("hello"), 0777) //preferred

// A MapFS is a simple in-memory file system for use in tests,
// represented as a map from path names (arguments to Open)
// to information about the files or directories they represent.
//
// The map need not include parent directories for files contained
// in the map; those will be synthesized if needed.
// But a directory can still be included by setting the MapFile.Mode's ModeDir bit;
// this may be necessary for detailed control over the directory's FileInfo
// or to create an empty directory.
//
// File system operations read directly from the map,
// so that the file system can be changed by editing the map as needed.
// An implication is that file system operations must not run concurrently
// with changes to the map, which would be a race.
// Another implication is that opening or reading a directory requires
// iterating over the entire map, so a MapFS should typically be used with not more
// than a few hundred entries or directory reads.
type MapFS map[string]*MapFile

// A mapDir is a directory fs.File (so also an fs.ReadDirFile) open for reading.
type mapDir struct {
	path string
	mapFileInfo
	entry  []mapFileInfo
	offset int
}

// An OpenMapFile is a regular (non-directory) fs.File open for reading (and now writing).
type OpenMapFile struct {
	path string
	mapFileInfo
	offset int64
}

// A mapFileInfo implements fs.FileInfo and fs.DirEntry for a given map file.
type mapFileInfo struct {
	name string
	f    *MapFile
}

// A MapFile describes a single file in a MapFS.
type MapFile struct {
	Data    []byte      // file content
	Mode    fs.FileMode // FileInfo.Mode
	ModTime time.Time   // FileInfo.ModTime
	Sys     any         // FileInfo.Sys
}

var _ fs.FS = MapFS(nil)
var _ fs.File = (*OpenMapFile)(nil)

// adds write method to fs.File
type FileWriter interface {
	fs.File
	Write(b []byte) (int, error)
}

// adds a Write method to fs.FS

// fsOnly is a wrapper that hides all but the fs.FS methods,
// to avoid an infinite recursion when implementing special
// methods in terms of helpers that would use them.
// (In general, implementing these methods using the package fs helpers
// is redundant and unnecessary, but having the methods may make
// MapFS exercise more code paths when used in tests.)
type fsOnly struct{ fs.FS }

func (fsys MapFS) ReadFile(name string) ([]byte, error) {
	//println("mapfs.go: MapFS.ReadFile", name, "--------------------------------------------------------------")
	return fs.ReadFile(fsOnly{fsys}, name)
}

func (fsys MapFS) Stat(name string) (fs.FileInfo, error) {
	//println("mapfs.go: MapFS.Stat", name, "--------------------------------------------------------------")
	//t := reflect.TypeOf(fsys) // get the type of the object
	//println("mapfs.go: MapFS.Stat Open:  type=", t.String())
	//println("----------------------------------")

	return fs.Stat(fsOnly{fsys}, name)
}

func (fsys MapFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(fsOnly{fsys}, name)
}

func (fsys MapFS) Glob(pattern string) ([]string, error) {
	return fs.Glob(fsOnly{fsys}, pattern)
}

type noSub struct {
	MapFS
}

func (noSub) Sub() {} // not the fs.SubFS signature

func (fsys MapFS) Sub(dir string) (fs.FS, error) {
	return fs.Sub(noSub{fsys}, dir)
}

func (i *mapFileInfo) Name() string               { return i.name }
func (i *mapFileInfo) Size() int64                { return int64(len(i.f.Data)) }
func (i *mapFileInfo) Mode() fs.FileMode          { return i.f.Mode }
func (i *mapFileInfo) Type() fs.FileMode          { return i.f.Mode.Type() }
func (i *mapFileInfo) ModTime() time.Time         { return i.f.ModTime }
func (i *mapFileInfo) IsDir() bool                { return i.f.Mode&fs.ModeDir != 0 }
func (i *mapFileInfo) Sys() any                   { return i.f.Sys }
func (i *mapFileInfo) Info() (fs.FileInfo, error) { return i, nil }

func (f *OpenMapFile) Stat() (fs.FileInfo, error) { return &f.mapFileInfo, nil }

func (f *OpenMapFile) Close() error { return nil }

func (f *OpenMapFile) Read(b []byte) (int, error) {
	//println("mapfs.go: OpenMapFile.Read", f.Name(), len(f.f.Data), "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	//println("f.f.Data len=", len(f.f.Data))
	if f.offset >= int64(len(f.f.Data)) {
		return 0, io.EOF
	}
	if f.offset < 0 {
		return 0, &fs.PathError{Op: "read", Path: f.path, Err: fs.ErrInvalid}
	}

	n := copy(b, f.f.Data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

//type TFile struct {
//	file fs.File
//}

//func (f TFile) Close() error {
//	return f.file.Close()
//}

// func (f *MapFile) Write(b []byte) (int, error) {
// func (f *OpenMapFile) Write(b []byte) (int, error) {
func (f *OpenMapFile) Write(b []byte) (int, error) {
	println("mapfs.go: OpenMapFile.Write", f.Name(), len(f.f.Data), "--------------------------------------------------------------")

	//of :=f(*OpenMapFile)
	//f.file.
	//if file, ok := f.file.(*OpenMapFile); ok {
	n := copy(f.f.Data, b)
	if n < len(b) {
		f.f.Data = append(f.f.Data, b[n:]...)
	}
	//}
	return len(b), nil
	/*
			if f.offset >= int64(len(f.f.Data)) {
				return 0, io.EOF //fs.ErrPerm
			}
			if f.offset < 0 {
				return 0, &fs.PathError{Op: "write", Path: f.path, Err: fs.ErrInvalid}
			}
			n := copy(f.f.Data[f.offset:], b)
			f.offset += int64(n)
		return n, nil
	*/
}
func (f *OpenMapFile) MapFile() *MapFile {
	return f.mapFileInfo.f
}
func (f *OpenMapFile) Name() string {
	return f.mapFileInfo.name
}
func (f *OpenMapFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		// offset += 0
	case 1:
		offset += f.offset
	case 2:
		offset += int64(len(f.f.Data))
	}
	if offset < 0 || offset > int64(len(f.f.Data)) {
		return 0, &fs.PathError{Op: "seek", Path: f.path, Err: fs.ErrInvalid}
	}
	f.offset = offset
	return offset, nil
}

func (f *OpenMapFile) ReadAt(b []byte, offset int64) (int, error) {
	if offset < 0 || offset > int64(len(f.f.Data)) {
		return 0, &fs.PathError{Op: "read", Path: f.path, Err: fs.ErrInvalid}
	}
	n := copy(b, f.f.Data[offset:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

func (d *mapDir) Stat() (fs.FileInfo, error) { return &d.mapFileInfo, nil }
func (d *mapDir) Close() error               { return nil }
func (d *mapDir) Read(b []byte) (int, error) {
	//println("mapfs.go: mapDir.Read", d.Name(), len(d.f.Data), "##########################################################")
	//println("d.f.Data len=", len(d.f.Data))
	return 0, &fs.PathError{Op: "read", Path: d.path, Err: fs.ErrInvalid}
}

func (d *mapDir) ReadDir(count int) ([]fs.DirEntry, error) {
	//println("mapfs.go: mapDir.ReadDir", d.Name(), len(d.f.Data), "--------------------------------------------------------------")
	n := len(d.entry) - d.offset
	if n == 0 && count > 0 {
		return nil, io.EOF
	}
	if count > 0 && n > count {
		n = count
	}
	list := make([]fs.DirEntry, n)
	for i := range list {
		list[i] = &d.entry[d.offset+i]
	}
	d.offset += n
	return list, nil
}

///////////////////////////////////////////////////////////////
// new MapFS functions here
///////////////////////////////////////////////////////////////

// perm is unused, but you need to pass in something, like 0777
func (fsys MapFS) Mkdir(name string, perm fs.FileMode) error {
	fsys[name] = &MapFile{
		Mode: fs.ModeDir,
	}
	return nil
}

// Remove removes the named file or (empty) directory.
// If there is an error, it will be of type *PathError.
func (fsys MapFS) Remove(name string) error {
	if name[0] == '/' {
		name = name[1:] // FS filesystem in go cannot start with /
	}
	delete(fsys, name)
	return nil
}

// Rename renames (moves) oldpath to newpath.
// If newpath already exists and is not a directory, Rename replaces it.
// OS-specific restrictions may apply when oldpath and newpath are in different directories.
// If there is an error, it will be of type *LinkError.
func (fsys MapFS) Rename(oldname, newname string) error {
	if oldname[0] == '/' {
		oldname = oldname[1:] // FS filesystem in go cannot start with /
	}
	if newname[0] == '/' {
		newname = newname[1:] // FS filesystem in go cannot start with /
	}
	fsys[newname] = fsys[oldname]
	delete(fsys, oldname)
	return nil
}

// Copy copies the file in src to dst.
func (fsys MapFS) Copy(dst, src string) error {
	srcFile := fsys[src]
	if srcFile == nil {
		return &fs.PathError{Op: "copy", Path: src, Err: fs.ErrNotExist}
	}
	fsys[dst] = srcFile
	return nil
}

/////////////////////////////////////////////////////////////////////////////////

// WriteFile writes data to a file named by filename. perm is not used but cn be set to
func (fsys MapFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	//println("mapfs.go: MapFS.WriteFile", name, len(data), "--------------------------------------------------------------")

	//perm is not implimented
	if name[0] == '/' {
		name = name[1:] // FS filesystem in go cannot start with /
	}
	fsys[name] = &MapFile{
		Data:    data,
		Mode:    perm,
		ModTime: time.Now(),
	}
	return nil
}

// Create a new file with the specified name and permission bits (before umask).
// If there is an error, it will be of type *PathError.
// func (fsys MapFS) Create(name string) (OpenMapFile, error) {
func (fsys MapFS) Create(name string) (fs.File, error) {
	println("mapfs.go: MapFS.Create", name, "--------------------------------------------------------------")

	if name[0] == '/' {
		name = name[1:] // FS filesystem cannot start with /
	}
	//mfi := mapFileInfo{
	//	name: name,
	//	f:    fsys[name],
	//}
	fsys[name] = &MapFile{
		Data:    []byte{},
		Mode:    0666, //perm is not implimented
		ModTime: time.Now(),
	}
	// Ordinary file
	return &OpenMapFile{name, mapFileInfo{path.Base(name), fsys[name]}, 0}, nil
	//return &OpenMapFile{path: name, mapFileInfo: mfi}, nil
}

// Open opens the named file.
func (fsys MapFS) Open(name string) (fs.File, error) {
	//println("mapfs.go: MapFS.Open", name, "[------------------------------------------------------------]")

	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	file, _ := fsys[name]
	//if ok {
	//	println("Open:  file found")
	//} else {
	//	println("Open:  file NOT found")
	//}
	if file != nil && file.Mode&fs.ModeDir == 0 {
		// Ordinary file
		t := reflect.TypeOf(file) // get the type of the object
		println("Open:  type=", t.String())
		println("#----------------------------------#")
		return &OpenMapFile{name, mapFileInfo{path.Base(name), file}, 0}, nil
	}

	// Directory, possibly synthesized.
	// Note that file can be nil here: the map need not contain explicit parent directories for all its files.
	// But file can also be non-nil, in case the user wants to set metadata for the directory explicitly.
	// Either way, we need to construct the list of children of this directory.
	var list []mapFileInfo
	var elem string
	var need = make(map[string]bool)
	if name == "." {
		//println("Open:  name == . (dot)")
		elem = "."
		for fname, f := range fsys {
			i := strings.Index(fname, "/")
			if i < 0 {
				if fname != "." {
					list = append(list, mapFileInfo{fname, f})
				}
			} else {
				need[fname[:i]] = true
			}
		}
	} else {
		//println("Open:  name != . (dot)")
		elem = name[strings.LastIndex(name, "/")+1:]
		prefix := name + "/"
		for fname, f := range fsys {
			// print if the file name starts with the prefix
			//if strings.HasPrefix(fname, prefix) {
			//	println("Open:  fname=", fname, "  prefix=", prefix)
			//}
			if strings.HasPrefix(fname, prefix) {
				felem := fname[len(prefix):]
				i := strings.Index(felem, "/")
				if i < 0 {
					list = append(list, mapFileInfo{felem, f})
				} else {
					need[fname[len(prefix):len(prefix)+i]] = true
				}
			}
		}
		//println("Open:  elem=", elem, "  len(list)=", len(list), "  len(need)=", len(need))
		// If the directory name is not in the map,
		// and there are no children of the name in the map,
		// then the directory is treated as not existing.
		if file == nil && list == nil && len(need) == 0 {
			//println("DIRECTORY NOT FOUND (1)", name)
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		} else {
			//println("DIRECTORY FOUND (1)", name)
		}
	}
	for _, fi := range list {
		//println("Open:  list:  fi.name=", fi.name, "  fi.f=", fi.f)
		delete(need, fi.name)
	}
	for name := range need {
		//println("Open:  need:  name=", name)
		list = append(list, mapFileInfo{name, &MapFile{Mode: fs.ModeDir}})
	}
	sort.Slice(list, func(i, j int) bool {
		//println("Open:  sort:  i=", i, "  j=", j, "  list[i].name=", list[i].name, "  list[j].name=", list[j].name)
		return list[i].name < list[j].name
	})
	//println("Open:  len(list)=", len(list))
	if file == nil {
		//println("file==nil")
		file = &MapFile{Mode: fs.ModeDir}
	}
	//println("TRUTH Open:  name", name)
	ff := &mapDir{name, mapFileInfo{elem, file}, list, 0}
	ff.name = name             //must set this or else it will be filename only without the path
	ff.mapFileInfo.name = name //same here
	//println("TRUTH Open:  ff.name", ff.name)
	//, "ff.mapFileInfo.name=", ff.mapFileInfo.name)
	return ff, nil
}
