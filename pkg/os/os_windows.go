// +build windows

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package os

import (
	"os"

	"golang.org/x/sys/windows"
)

// openPath takes a path, opens it, and returns the resulting handle.
// It works for both file and directory paths.
//
// We are not able to use builtin Go functionality for opening a directory path:
// - os.Open on a directory returns a os.File where Fd() is a search handle from FindFirstFile.
// - syscall.Open does not provide a way to specify FILE_FLAG_BACKUP_SEMANTICS, which is needed to
//   open a directory.
// We could use os.Open if the path is a file, but it's easier to just use the same code for both.
// Therefore, we call windows.CreateFile directly.
func openPath(path string) (windows.Handle, error) {
	u16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(
		u16,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS, // Needed to open a directory handle.
		0)
	if err != nil {
		return 0, &os.PathError{
			Op:   "CreateFile",
			Path: path,
			Err:  err,
		}
	}
	return h, nil
}

// GetFinalPathNameByHandle flags.
const (
	cFILE_NAME_NORMALIZED = 0x0
	cFILE_NAME_OPENED     = 0x8

	cVOLUME_NAME_DOS  = 0x0
	cVOLUME_NAME_GUID = 0x1
	cVOLUME_NAME_NONE = 0x4
	cVOLUME_NAME_NT   = 0x2
)

// getFinalPathNameByHandle facilitates calling the Windows API GetFinalPathNameByHandle
// with the given handle and flags. It transparently takes care of creating a buffer of the
// correct size for the call.
func getFinalPathNameByHandle(h windows.Handle, flags uint32) (string, error) {
	n, err := windows.GetFinalPathNameByHandle(h, nil, 0, flags)
	if err != nil {
		return "", err
	}
	b := make([]uint16, n)
	_, err = windows.GetFinalPathNameByHandle(h, &b[0], n, flags)
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(b), nil
}

// resolvePath implements path resolution for Windows. It attempts to return the "real" path to the
// file or directory represented by the given path.
// The resolution works by using the Windows API GetFinalPathNameByHandle, which takes a handle and
// returns the final path to that file.
func resolvePath(path string) (string, error) {
	h, err := openPath(path)
	if err != nil {
		return "", err
	}
	rPath, err := getFinalPathNameByHandle(h, cFILE_NAME_OPENED|cVOLUME_NAME_DOS)
	if err == windows.ERROR_PATH_NOT_FOUND {
		// ERROR_PATH_NOT_FOUND is returned from the VOLUME_NAME_DOS query if the path is a
		// volume GUID path to a disk, and the disk does not have an assigned drive letter.
		// In this case, just get the volume GUID path instead.
		rPath, err = getFinalPathNameByHandle(h, cFILE_NAME_OPENED|cVOLUME_NAME_GUID)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return rPath, nil
}

// ResolveSymbolicLink will follow any symbolic links
func (RealOS) ResolveSymbolicLink(path string) (string, error) {
	// filepath.EvalSymlinks does not work very well on Windows, so instead we resolve the path
	// via resolvePath which uses GetFinalPathNameByHandle. This returns a path prefixed with `\\?\`,
	// which should work with most Go APIs.
	return resolvePath(path)
}
