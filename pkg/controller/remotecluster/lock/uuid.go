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

package lock

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

type UUIDLock interface {
	LockByOwner(uuid types.UID, owner string) error
	Drop(uuid types.UID)
	DropByOwner(owner string)
}

type uuidLock struct {
	sync.Mutex
	uuidMap map[types.UID]string
}

func (u *uuidLock) LockByOwner(uuid types.UID, owner string) error {
	u.Lock()
	defer u.Unlock()

	if currentOwner, ok := u.uuidMap[uuid]; ok {
		if currentOwner != owner {
			return fmt.Errorf("uuid %s is already locked by owner %s", uuid, currentOwner)
		}
	} else {
		u.uuidMap[uuid] = owner
	}
	return nil
}

func (u *uuidLock) Drop(uuid types.UID) {
	u.Lock()
	defer u.Unlock()

	delete(u.uuidMap, uuid)
}

func (u *uuidLock) DropByOwner(owner string) {
	u.Lock()
	defer u.Unlock()

	var dropUUIDs []types.UID
	for uuid, currentOwner := range u.uuidMap {
		if currentOwner == owner {
			dropUUIDs = append(dropUUIDs, uuid)
		}
	}

	for _, dropUUID := range dropUUIDs {
		delete(u.uuidMap, dropUUID)
	}
}

func NewUUIDLock() UUIDLock {
	return &uuidLock{
		Mutex:   sync.Mutex{},
		uuidMap: make(map[types.UID]string),
	}
}
