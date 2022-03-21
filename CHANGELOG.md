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
