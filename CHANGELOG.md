# Hybridnet Changelog
All notable changes to this project will be documented in this file.

## v0.1.0
### New features
- Support overlay (vxlan) network
- Support hybrid overlay/underlay container network
- Full support for ipv4/ipv6 dual-stack

### Improvements
- Node need only one physical nic if container network is in the same vlan with node network
- Non-zero-netId subnet and zero-netId subnet can be on the same node
- Webhook configuration can be managed by an independent yaml
- Use default-ip-retain global flag and ip-retain pod annotation to reallocate/retain IP

### Fixed Issues
- Remove overlay logs for underlay-only mode
- Fix error of using prefer interfaces list
- Fix timeout error of pod creation on large scale

## v0.1.1
### Improvements
- Add checks for pod using the same subnet with node
- Support setting linux kernel neigh gc thresh parameters
- Only choose vtep and node ip as node internal overlay container networking ip, support extra selection
- Remove duplicated routes
- Adapt to underlay physical environment with arp sender ip check
- Add prechecking for check pod network configuration, if not ready, pod will not be created successfully

### Fixed Issues
- Fix error data path for overlay pod to access underlay gateway and excluded ip addresses

## v0.1.2
### Improvements
- Clear stale neigh entries for overlay network

## v0.2.0
### New features
- Change project name to "hybridnet", which is completely forward-compatible

### Improvements
- Network type will be auto selected while pod has a specified network

### Fixed Issues
- Fix wrong masquerading for remote pod to access local pod (update daemon image and rebuild pod will take effect)

## v0.2.1
### Fixed Issues
- Fix daemon iptables-restore execute error on CentOS 8 

## v0.3.0
### New features
- Support multicluster feature, which can connect the network between the two clusters (pod ip only)

### Improvements
- Recycle IP instances for Completed or Evicted pods
- Use controller-gen to generate crd ini yaml file

### Fixed Issues
- Fix masquerade error sometimes overlay pod access to underlay pod
- Fix high CPU cost of hybridnet daemon in large scale cluster
- Fix wrong underlay pod scheduling if not all the nodes belong to an underlay network while an overlay network exists

## v0.3.1
### Improvements
- Detect OS parameters for disabling IPv6-related operations
- Disallow unexpected CIDR notation in APIs

### Fixed Issues
- Avoid permanent exit of arp proxy on large-scale clusters

## v0.3.2
### Improvements
- Short-circuit terminating pods before enqueuing in manager controller

### Fixed Issues
- Fix ipv6 address range calculation error
- Fix nil point dereference error while creating a vlan interface

## v0.3.3
### Fixed Issues
- Introduce flag `enable-vlan-arp-enhancement` to disable setting enhanced addresses by default

## v0.3.4
### Fixed Issues
- Prevent enhanced addresses from source selection

## v0.4.0
### New features
- Support BGP mode for an Underlay type Network
- Support specifying namespace with network/subnet/network-type/ip-family
- Introduce Felix for NetworkPolicy

### Improvements
- Refactor daemon/manager/webhook with controller-runtime
- Deny the creation of /32 or /128 Subnets in webhook
- Only IPv4 feature valid if DualStack feature-gate is false
- Specify subnet without a specified Network
- Gateway field becomes optional for VXLAN/BGP Subnets

### Fixed Issues
- Fix specifying subnets error for DualStack pod
- Fix updating failure of nodes' vxlan fdb for a new node

## v0.4.1
### Improvements
- Adjust client QPS and Burst configuration of manager
- Mute useless logs of manager

### Fixed Issues
- Fix "file exists" error while creating pod

## v0.4.2
### Fixed Issues
- Fix creating ip-retained sts pod error when it is recreated and rescheduled to another node

## v0.4.3
### Improvements
- Introduce flag `enable-vlan-arp-enhancement` for daemon to enable/disable enhanced addresses
- Introduce `DEFAULT_IP_FAMILY` environment variable on dual-stack mode
- Skip webhook validation on host-networking pods
- Introduce `vtep-address-cidrs` flag for daemon to help select vtep address

### Fixed Issues
- Fix daemon policy container init error on ipv6-only node
- Node annotation changed should trigger the reconcile of daemon Node controller
- Fix "to overlay subnet route table 40000 is used by others" error of daemon. It happens if an ipv6 subnet with excluded ip ranges is created
- Fix daemon update dual-stack IPInstance status error
- Fix the error that arp enhanced addresses will be taken as source IP address by mistake
- Fix the error that deprecated bgp rules and routes are not cleaned

## v0.4.4
### Fixed Issues
- Fix the error that nodes get "empty" quota while the Underlay Network still have available addresses to allocate
- Fix daemon policy container exit with ip6tables-legacy-save error

## v0.5.0
### New features
- Change IPInstance APIs and optimize IP allocation performance of manager
- Introduce GlobalBGP type Network

### Improvements
- Bump controller-runtime from v0.8.3 to v0.9.7

## v0.5.1
### Fixed Issues
- Fix address duplication error while active-standby switch of manager pods happens

## v0.6.0
### New features
- Remove DualStack feature gate and make it built in
- Support to retain ip for kubevirt VMs

### Improvements
- Bump golang from v1.16 to v1.17
- Add limitations for creating overlapped subnets
- Disable the automatic iptables mode detection of felix
- Print statistics for Network CR
