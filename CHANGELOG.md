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
