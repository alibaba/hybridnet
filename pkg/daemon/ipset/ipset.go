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

package ipset

/*
  Copyright 2004 The Kube-router Authors.

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

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"sync"
)

var (
	// Error returned when ipset binary is not found.
	errIpsetNotFound = errors.New("ipset utility not found")
)

const (
	// FamillyInet IPV4.
	FamillyInet = "inet"
	// FamillyInet6 IPV6.
	FamillyInet6 = "inet6"

	// DefaultMaxElem Default OptionMaxElem value.
	DefaultMaxElem = "65536"
	// DefaultHasSize Defaul OptionHashSize value.
	DefaultHasSize = "1024"

	// TypeBitmapPort The bitmap:port set type uses a bitmap to store port numbers
	TypeBitmapPort = "bitmap:port"
	// TypeHashIP The hash:ip set type uses a hash to store IP host addresses (default) or network addresses. Zero valued IP address cannot be stored in a hash:ip type of set.
	TypeHashIP = "hash:ip"
	// TypeHashMac The hash:mac set type uses a hash to store MAC addresses. Zero valued MAC addresses cannot be stored in a hash:mac type of set.
	TypeHashMac = "hash:mac"
	// TypeHashNet The hash:net set type uses a hash to store different sized IP network addresses. Network address with zero prefix size cannot be stored in this type of sets.
	TypeHashNet = "hash:net"
	// TypeHashNetNet The hash:net,net set type uses a hash to store pairs of different sized IP network addresses. Bear in mind that the first parameter has precedence over the second, so a nomatch entry could be potentially be ineffective if a more specific first parameter existed with a suitable second parameter. Network address with zero prefix size cannot be stored in this type of set.
	TypeHashNetNet = "hash:net,net"
	// TypeHashIPPort The hash:ip,port set type uses a hash to store IP address and port number pairs. The port number is interpreted together with a protocol (default TCP) and zero protocol number cannot be used.
	TypeHashIPPort = "hash:ip,port"
	// TypeHashNetPort The hash:net,port set type uses a hash to store different sized IP network address and port pairs. The port number is interpreted together with a protocol (default TCP) and zero protocol number cannot be used. Network address with zero prefix size is not accepted either.
	TypeHashNetPort = "hash:net,port"
	// TypeHashIPPortIP The hash:ip,port,ip set type uses a hash to store IP address, port number and a second IP address triples. The port number is interpreted together with a protocol (default TCP) and zero protocol number cannot be used.
	TypeHashIPPortIP = "hash:ip,port,ip"
	// TypeHashIPPortNet The hash:ip,port,net set type uses a hash to store IP address, port number and IP network address triples. The port number is interpreted together with a protocol (default TCP) and zero protocol number cannot be used. Network address with zero prefix size cannot be stored either.
	TypeHashIPPortNet = "hash:ip,port,net"
	// TypeHashIPMark The hash:ip,mark set type uses a hash to store IP address and packet mark pairs.
	TypeHashIPMark = "hash:ip,mark"
	// TypeHashIPNetPortNet The hash:net,port,net set type behaves similarly to hash:ip,port,net but accepts a cidr value for both the first and last parameter. Either subnet is permitted to be a /0 should you wish to match port between all destinations.
	TypeHashIPNetPortNet = "hash:net,port,net"
	// TypeHashNetIface The hash:net,iface set type uses a hash to store different sized IP network address and interface name pairs.
	TypeHashNetIface = "hash:net,iface"
	// TypeListSet The list:set type uses a simple list in which you can store set names.
	TypeListSet = "list:set"

	// OptionTimeout All set types supports the optional timeout parameter when creating a set and adding entries. The value of the timeout parameter for the create command means the default timeout value (in seconds) for new entries. If a set is created with timeout support, then the same timeout option can be used to specify non-default timeout values when adding entries. Zero timeout value means the entry is added permanent to the set. The timeout value of already added elements can be changed by readding the element using the -exist option. When listing the set, the number of entries printed in the header might be larger than the listed number of entries for sets with the timeout extensions: the number of entries in the set is updated when elements added/deleted to the set and periodically when the garbage colletor evicts the timed out entries.`
	OptionTimeout = "timeout"
	// OptionCounters All set types support the optional counters option when creating a set. If the option is specified then the set is created with packet and byte counters per element support. The packet and byte counters are initialized to zero when the elements are (re-)added to the set, unless the packet and byte counter values are explicitly specified by the packets and bytes options. An example when an element is added to a set with non-zero counter values.
	OptionCounters = "counters"
	// OptionPackets All set types support the optional counters option when creating a set. If the option is specified then the set is created with packet and byte counters per element support. The packet and byte counters are initialized to zero when the elements are (re-)added to the set, unless the packet and byte counter values are explicitly specified by the packets and bytes options. An example when an element is added to a set with non-zero counter values.
	OptionPackets = "packets"
	// OptionBytes All set types support the optional counters option when creating a set. If the option is specified then the set is created with packet and byte counters per element support. The packet and byte counters are initialized to zero when the elements are (re-)added to the set, unless the packet and byte counter values are explicitly specified by the packets and bytes options. An example when an element is added to a set with non-zero counter values.
	OptionBytes = "bytes"
	// OptionComment All set types support the optional comment extension. Enabling this extension on an ipset enables you to annotate an ipset entry with an arbitrary string. This string is completely ignored by both the kernel and ipset itself and is purely for providing a convenient means to document the reason for an entry's existence. Comments must not contain any quotation marks and the usual escape character (\) has no meaning
	OptionComment = "comment"
	// OptionSkbinfo All set types support the optional skbinfo extension. This extension allow to store the metainfo (firewall mark, tc class and hardware queue) with every entry and map it to packets by usage of SET netfilter target with --map-set option. skbmark option format: MARK or MARK/MASK, where MARK and MASK are 32bit hex numbers with 0x prefix. If only mark is specified mask 0xffffffff are used. skbprio option has tc class format: MAJOR:MINOR, where major and minor numbers are hex without 0x prefix. skbqueue option is just decimal number.
	OptionSkbinfo = "skbinfo"
	// OptionSkbmark All set types support the optional skbinfo extension. This extension allow to store the metainfo (firewall mark, tc class and hardware queue) with every entry and map it to packets by usage of SET netfilter target with --map-set option. skbmark option format: MARK or MARK/MASK, where MARK and MASK are 32bit hex numbers with 0x prefix. If only mark is specified mask 0xffffffff are used. skbprio option has tc class format: MAJOR:MINOR, where major and minor numbers are hex without 0x prefix. skbqueue option is just decimal number.
	OptionSkbmark = "skbmark"
	// OptionSkbprio All set types support the optional skbinfo extension. This extension allow to store the metainfo (firewall mark, tc class and hardware queue) with every entry and map it to packets by usage of SET netfilter target with --map-set option. skbmark option format: MARK or MARK/MASK, where MARK and MASK are 32bit hex numbers with 0x prefix. If only mark is specified mask 0xffffffff are used. skbprio option has tc class format: MAJOR:MINOR, where major and minor numbers are hex without 0x prefix. skbqueue option is just decimal number.
	OptionSkbprio = "skbprio"
	// OptionSkbqueue All set types support the optional skbinfo extension. This extension allow to store the metainfo (firewall mark, tc class and hardware queue) with every entry and map it to packets by usage of SET netfilter target with --map-set option. skbmark option format: MARK or MARK/MASK, where MARK and MASK are 32bit hex numbers with 0x prefix. If only mark is specified mask 0xffffffff are used. skbprio option has tc class format: MAJOR:MINOR, where major and minor numbers are hex without 0x prefix. skbqueue option is just decimal number.
	OptionSkbqueue = "skbqueue"
	// OptionHashSize This parameter is valid for the create command of all hash type sets. It defines the initial hash size for the set, default is 1024. The hash size must be a power of two, the kernel automatically rounds up non power of two hash sizes to the first correct value.
	OptionHashSize = "hashsize"
	// OptionMaxElem This parameter is valid for the create command of all hash type sets. It does define the maximal number of elements which can be stored in the set, default 65536.
	OptionMaxElem = "maxelem"
	// OptionFamilly This parameter is valid for the create command of all hash type sets except for hash:mac. It defines the protocol family of the IP addresses to be stored in the set. The default is inet, i.e IPv4.
	OptionFamilly = "family"
	// OptionNoMatch The hash set types which can store net type of data (i.e. hash:*net*) support the optional nomatch option when adding entries. When matching elements in the set, entries marked as nomatch are skipped as if those were not added to the set, which makes possible to build up sets with exceptions. See the example at hash type hash:net below. When elements are tested by ipset, the nomatch flags are taken into account. If one wants to test the existence of an element marked with nomatch in a set, then the flag must be specified too.
	OptionNoMatch = "nomatch"
	// OptionForceAdd All hash set types support the optional forceadd parameter when creating a set. When sets created with this option become full the next addition to the set may succeed and evict a random entry from the set.
	OptionForceAdd = "forceadd"
)

// An injectable interface for running ipset commands.  Implementations must be goroutine-safe.
type Interface interface {
	// AddOrReplaceIPSet add or replace an exist ipset with given members and createOptions.
	AddOrReplaceIPSet(setName string, members []string, createOptions ...string)
	// AddOrReplaceIPSetWithBuiltinOptions extends AddOrReplaceIPSet with createOptions.
	AddOrReplaceIPSetWithBuiltinOptions(setName string, members [][]string, createOptions ...string)
	// RemoveIPSet remove an exist ipset.
	RemoveIPSet(setName string)
	// ListIPSets list current set list.
	ListIPSets() []string
	// LoadData calls `ipset save` and save information to Sets.
	LoadData() error
	// SyncOperations runs `ipset restore`.
	SyncOperations() error
}

type Set struct {
	Name    string
	Entries []*Entry
	Options []string
}

type Entry struct {
	Options []string
}

// runner implements Interface in terms of exec("iptables").
type runner struct {
	mu        sync.Mutex
	ipSetPath string

	Sets     map[string]*Set
	commands *bytes.Buffer

	isIpv6 bool
}

// New returns a new Interface which will exec iptables.
func New(isIpv6 bool) (Interface, error) {
	ipSetPath, err := getIPSetPath()
	if err != nil {
		return nil, err
	}
	runner := newInternal(isIpv6, *ipSetPath)
	return runner, nil
}

func (r *runner) LoadData() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	stdout, err := r.run("save")
	if err != nil {
		return err
	}
	r.parseIPSetSave(stdout)
	return nil
}

func (r *runner) AddOrReplaceIPSet(setName string, members []string, createOptions ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	setToCreate := setName
	needSwap := false
	if _, exist := r.Sets[setName]; exist {
		// create an ipset
		setToCreate = setName + "-tmp"
		needSwap = true
	}

	args := append([]string{"create", setToCreate}, createOptions...)
	if r.isIpv6 {
		args = append(args, "family", FamillyInet6)
	}
	r.recordCommand(args...)

	for _, member := range members {
		r.recordCommand("add", setToCreate, member)
	}

	if needSwap {
		// Atomically swap the temporary set into place.
		r.recordCommand("swap", setToCreate, setName)
		// Then remove the temporary set (which was the old main set).
		r.recordCommand("destroy", setToCreate)
	}
}

func (r *runner) AddOrReplaceIPSetWithBuiltinOptions(setName string, members [][]string, createOptions ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	setToCreate := setName
	needSwap := false
	if _, exist := r.Sets[setName]; exist {
		// create an ipset
		setToCreate = setName + "-tmp"
		needSwap = true
	}

	args := append([]string{"create", setToCreate}, createOptions...)
	if r.isIpv6 {
		args = append(args, "family", "inet6")
	}
	r.recordCommand(args...)

	for _, member := range members {
		r.recordCommand(append([]string{"add", setToCreate}, member...)...)
	}

	if needSwap {
		// Atomically swap the temporary set into place.
		r.recordCommand("swap", setToCreate, setName)
		// Then remove the temporary set (which was the old main set).
		r.recordCommand("destroy", setToCreate)
	}
}

func (r *runner) RemoveIPSet(setName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.Sets[setName]; exist {
		r.recordCommand("destroy", setName)
	}
}

func (r *runner) ListIPSets() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var setList []string
	for set := range r.Sets {
		setList = append(setList, set)
	}
	return setList
}

func (r *runner) SyncOperations() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.runWithStdin(r.commands, "restore", "-exist")
	if err != nil {
		return err
	}
	return nil
}

// newInternal returns a new Interface which will exec iptables, and allows the
// caller to change the iptables-restore lockfile path
func newInternal(isIpv6 bool, ipSetPath string) Interface {
	runner := &runner{
		ipSetPath: ipSetPath,
		isIpv6:    isIpv6,
		commands:  bytes.NewBuffer(nil),
		Sets:      make(map[string]*Set),
	}
	return runner
}

// Get ipset binary path or return an error.
func getIPSetPath() (*string, error) {
	path, err := exec.LookPath("ipset")
	if err != nil {
		return nil, errIpsetNotFound
	}
	return &path, nil
}

// Used to run ipset binary with args and return stdout.
func (r *runner) run(args ...string) (string, error) {
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd := exec.Cmd{
		Path:   r.ipSetPath,
		Args:   append([]string{r.ipSetPath}, args...),
		Stderr: &stderr,
		Stdout: &stdout,
	}

	if err := cmd.Run(); err != nil {
		return "", errors.New(stderr.String())
	}

	return stdout.String(), nil
}

// Used to run ipset binary with arg and inject stdin buffer and return stdout.
func (r *runner) runWithStdin(stdin *bytes.Buffer, args ...string) (string, error) {
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd := exec.Cmd{
		Path:   r.ipSetPath,
		Args:   append([]string{r.ipSetPath}, args...),
		Stderr: &stderr,
		Stdout: &stdout,
		Stdin:  stdin,
	}

	if err := cmd.Run(); err != nil {
		return "", errors.New(stderr.String())
	}

	return stdout.String(), nil
}

// Parse ipset save stdout.
// ex:
// create KUBE-DST-3YNVZWWGX3UQQ4VQ hash:ip family inet hashsize 1024 maxelem 65536 timeout 0
// add KUBE-DST-3YNVZWWGX3UQQ4VQ 100.96.1.6 timeout 0
func (r *runner) parseIPSetSave(result string) {
	sets := make(map[string]*Set)
	// Save is always in order
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		content := strings.Split(line, " ")
		if content[0] == "create" {
			sets[content[1]] = &Set{
				Name:    content[1],
				Options: content[2:],
			}
		} else if content[0] == "add" {
			set := sets[content[1]]
			set.Entries = append(set.Entries, &Entry{
				Options: content[2:],
			})
		}
	}
	r.Sets = sets
}

// Join all words with spaces, terminate with newline and write to commands.
func (r *runner) recordCommand(words ...string) {
	// We avoid strings.Join for performance reasonss.
	for i := range words {
		r.commands.WriteString(words[i])
		if i < len(words)-1 {
			r.commands.WriteByte(' ')
		} else {
			r.commands.WriteByte('\n')
		}
	}
}
