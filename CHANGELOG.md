# Rama Changelog
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