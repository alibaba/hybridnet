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

package multicluster

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

var _ UUIDMutex = &uuidMutex{}

type UUIDMutex interface {
	Lock(UUID types.UID, name string) (bool, error)
	Unlock(UUID types.UID) bool
	GetLatestUUID(name string) (bool, types.UID)
	GetUUIDs(name string) []types.UID
}

type uuidMutex struct {
	mutex           sync.Mutex
	idNameMap       map[types.UID]string
	nameLatestIDMap map[string]types.UID
}

func (u *uuidMutex) Lock(UUID types.UID, name string) (bool, error) {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	if len(UUID) == 0 {
		return false, fmt.Errorf("invalid empty UUID")
	}
	if currentName, exists := u.idNameMap[UUID]; exists {
		if currentName == name {
			return false, nil
		}
		return false, fmt.Errorf("UUID %s already locked by %s", UUID, currentName)
	}
	u.idNameMap[UUID] = name
	u.nameLatestIDMap[name] = UUID
	return true, nil
}

func (u *uuidMutex) Unlock(UUID types.UID) bool {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	if name, exists := u.idNameMap[UUID]; exists {
		delete(u.idNameMap, UUID)
		if uuid, exists := u.nameLatestIDMap[name]; exists && uuid == UUID {
			delete(u.nameLatestIDMap, name)
		}
		return true
	}
	return false
}

func (u *uuidMutex) GetLatestUUID(name string) (bool, types.UID) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	uuid, exists := u.nameLatestIDMap[name]
	return exists, uuid
}

func (u *uuidMutex) GetUUIDs(name string) []types.UID {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	ret := make([]types.UID, 0)
	for u, n := range u.idNameMap {
		if n == name {
			ret = append(ret, u)
		}
	}
	return ret
}

func NewUUIDMutex() UUIDMutex {
	return &uuidMutex{
		mutex:     sync.Mutex{},
		idNameMap: make(map[types.UID]string),
	}
}

func NewUUIDMutexFromClient(c client.Reader) (UUIDMutex, error) {
	mutex := NewUUIDMutex()
	localUUID, err := utils.GetClusterUUID(c)
	if err != nil {
		return nil, err
	}
	if _, err = mutex.Lock(localUUID, "LocalCluster"); err != nil {
		return nil, err
	}

	remoteClusterList, err := utils.ListRemoteClusters(c)
	if err != nil {
		return nil, err
	}
	for i := range remoteClusterList.Items {
		var remoteCluster = &remoteClusterList.Items[i]
		if len(remoteCluster.Status.UUID) > 0 {
			if _, err = mutex.Lock(remoteCluster.Status.UUID, remoteCluster.Name); err != nil {
				return nil, err
			}
		}
	}
	return mutex, nil
}
