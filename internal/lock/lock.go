/*
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

package lock

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"syscall"

	"k8s.io/klog"
)

const DirPermissions = os.ModeDir | 0775

var lockFile *os.File

func AcquireFileLock(filePath string) *os.File {
	lockFile, err := acquireOpenFileLock(filePath, syscall.LOCK_EX)
	if err != nil {
		klog.Errorf("Failed to rcquire lock for %v: %v", filePath, err)
		return nil
	}

	klog.Infof("Acquire lock [%v]", filePath)
	return lockFile
}

func ReleaseFileLock(file *os.File) {
	if file == nil {
		klog.Warningf("Failed to release lock for empty lock file")
		return
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		klog.Errorf("Failed to release lock for %v: %v", file.Name(), err)
	}

	if err := file.Close(); err != nil {
		klog.Errorf("Failed to close lock file %v: %v", file.Name(), err)
	}

	klog.Infof("Release lock [%v]", file.Name())
}

func acquireOpenFileLock(filePath string, how int) (*os.File, error) {
	lockFile, err := openLockFile(filePath)
	if err != nil {
		return nil, err
	}

	if err = acquireFileLock(lockFile, how); err != nil {
		return nil, err
	}

	return lockFile, nil
}

func openLockFile(filePath string) (*os.File, error) {
	lockFile, err := openDirFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("Failed to open %v to acquire lock: %v", filePath, err)
	}

	return lockFile, nil
}

func acquireFileLock(file *os.File, how int) error {
	err := syscall.Flock(int(file.Fd()), how|syscall.LOCK_NB)
	if err != nil {
		pid, err := ioutil.ReadFile(file.Name())
		if err == nil && len(pid) > 0 {
			klog.Warningf("Process with PID %v has already acquired the lock for %v. Waiting ...", string(pid), file.Name())
		} else {
			klog.Warningf("Another process has already acquired the lock for %v. Waiting ...", file.Name())
		}

		if err := syscall.Flock(int(file.Fd()), how); err != nil {
			return fmt.Errorf("Failed to acquire lock for %v: %v", file.Name(), err)
		}
	}

	return nil
}

func openDir(filename string) error {
	dir := path.Dir(filename)
	err := os.MkdirAll(dir, DirPermissions)
	if err != nil && IsFileExists(dir) {
		if err2 := os.Remove(dir); err2 == nil {
			err = os.MkdirAll(dir, DirPermissions)
		} else {
			klog.Errorf("Failed to create dir %v: %v", filename, err2)
		}
	}

	return err
}

func IsFileExists(filename string) bool {
	info, err := os.Lstat(filename)
	return err == nil && !info.IsDir()
}

func openDirFile(filename string, flag int, perm os.FileMode) (*os.File, error) {
	err := openDir(filename)
	if err != nil {
		return nil, err
	}

	return os.OpenFile(filename, flag, perm)
}

func CreateDirFile(filePath string) (*os.File, error) {
	file, err := openDirFile(filePath, os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("Failed to create %v: %v", filePath, err)
	}

	klog.Infof("File [%v] created successfully", file.Name())
	return file, nil
}

func RemoveFile(filePath string) error {
	if err := os.RemoveAll(filePath); err == nil || os.IsNotExist(err) {
		klog.Infof("File [%v] deleted successfully", filePath)
		return nil
	} else {
		return fmt.Errorf("Failed to delete %v: %v", filePath, err)
	}
}
