/*
 Copyright 2021 The Hybridnet Authors.

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

package utils

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/containernetworking/plugins/pkg/ns"
)

type HybridnetDaemonError string

func (e HybridnetDaemonError) Error() string {
	return "error info: " + string(e)
}

const (
	NotExist = HybridnetDaemonError("not exist")
)

func ValidDockerNetnsDir(path string) bool {
	defaultNS := path + "/" + "default"
	if _, err := os.Stat(defaultNS); err != nil {
		return false
	}
	return true
}

func IsProcFS(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return IsFs(path, ns.PROCFS_MAGIC)
}

func IsNsFS(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return IsFs(path, ns.NSFS_MAGIC)
}

func IsFs(path string, magic int64) bool {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		panic(os.NewSyscallError("statfs", err))
	}
	return stat.Type == magic
}

// SetSysctl modifies the specified sysctl flag to the new value
func SetSysctl(sysctlPath string, newVal int) error {
	return ioutil.WriteFile(sysctlPath, []byte(strconv.Itoa(newVal)), 0640)
}

func SetSysctlIgnoreNotExist(sysctlPath string, newVal int) error {
	err := ioutil.WriteFile(sysctlPath, []byte(strconv.Itoa(newVal)), 0640)

	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// GetSysctl modifies the specified sysctl flag to the new value
func GetSysctl(sysctlPath string) (int, error) {
	data, err := ioutil.ReadFile(sysctlPath)
	if err != nil {
		return -1, err
	}

	val, err := strconv.Atoi(strings.Trim(string(data), " \n"))
	if err != nil {
		return -1, err
	}

	return val, nil
}
