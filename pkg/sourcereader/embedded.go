// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sourcereader

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

// ModuleFS contains embedded modules (./modules) for use in building
// blueprints. The main package creates and injects the modules directory as
// hpc-toolkit/modules are not accessible at the package level.
var ModuleFS BaseFS

// BaseFS is an extension of the io.fs interface with the functionality needed
// in CopyDirFromModules. Works with embed.FS and afero.FS
type BaseFS interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
}

// EmbeddedSourceReader reads modules from a local directory
type EmbeddedSourceReader struct{}

// copyDirFromModules copies an FS directory to a local path
func copyDirFromModules(fs BaseFS, source string, dest string) error {
	dirEntries, err := fs.ReadDir(source)
	if err != nil {
		return err
	}
	for _, dirEntry := range dirEntries {
		entryName := dirEntry.Name()
		// path package (not path/filepath) should be used for embedded source
		// as the path separator is a forward slash, even on Windows systems.
		// https://pkg.go.dev/embed#hdr-Directives
		entrySource := path.Join(source, entryName)
		entryDest := filepath.Join(dest, entryName)
		if dirEntry.IsDir() {
			if err := os.Mkdir(entryDest, 0755); err != nil {
				return err
			}
			if err = copyDirFromModules(fs, entrySource, entryDest); err != nil {
				return err
			}
		} else {
			fileBytes, err := fs.ReadFile(entrySource)
			if err != nil {
				return err
			}
			copyFile, err := os.Create(entryDest)
			if err != nil {
				return err
			}
			if _, err = copyFile.Write(fileBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFSToTempDir is a temporary workaround until tfconfig.ReadFromFilesystem
// works against embed.FS.
// Open Issue: https://github.com/hashicorp/terraform-config-inspect/issues/68
func copyFSToTempDir(fs BaseFS, modulePath string) (string, error) {
	tmpDir, err := ioutil.TempDir("", "tfconfig-module-*")
	if err != nil {
		return tmpDir, err
	}
	err = copyDirFromModules(fs, modulePath, tmpDir)
	return tmpDir, err
}

// GetModule copies the embedded source to a provided destination (the deployment directory)
func (r EmbeddedSourceReader) GetModule(modPath string, copyPath string) error {
	if !IsEmbeddedPath(modPath) {
		return fmt.Errorf("Source is not valid: %s", modPath)
	}

	modDir, err := copyFSToTempDir(ModuleFS, modPath)
	defer os.RemoveAll(modDir)
	if err != nil {
		err = fmt.Errorf("failed to copy embedded module at %s to tmp dir %s: %v",
			modPath, modDir, err)
		return err
	}

	return copyFromPath(modDir, copyPath)
}
