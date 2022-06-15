# Hybridnet

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/hybridnet)](https://goreportcard.com/report/github.com/alibaba/hybridnet)
[![Github All Releases](https://img.shields.io/docker/pulls/hybridnetdev/hybridnet.svg)](https://hub.docker.com/r/hybridnetdev/hybridnet/tags)
[![Version](https://img.shields.io/github/v/release/alibaba/hybridnet)](https://github.com/alibaba/hybridnet/releases)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/hybridnet)](https://artifacthub.io/packages/search?repo=hybridnet)
[![License](https://img.shields.io/github/license/alibaba/hybridnet)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![codecov](https://codecov.io/gh/alibaba/hybridnet/branch/main/graphs/badge.svg)](https://codecov.io/gh/alibaba/hybridnet)
![workflow check](https://github.com/alibaba/hybridnet/actions/workflows/check.yml/badge.svg)
![workflow build](https://github.com/alibaba/hybridnet/actions/workflows/build.yml/badge.svg)

Hybridnet is an open source container networking solution designed for hybrid clouds, integrated with Kubernetes and used officially by following well-known PaaS platforms

- ACK Distro of Alibaba Cloud
- AECP of Alibaba Cloud
- SOFAStack of Ant Financial Co.


## Introduction

Most CNI plugins classify the forms of container network into two types and make them work at their own paces without connection:
1. *Overlay*, an abstract data plane on top of host network which is usually not visible to underlying network and brings cost
2. *Underlay*, putting container traffic "directly" into host network and depending on the abilities of underlying network

In hybridnet, we try to break the strict boundary of all the forms of container network with a simple design:
1. Overlay and underlay network can be created in the same cluster
2. If a connection has either an overlay container side (even the other side is an underlay container), it's considered an "Overlay" connection (without NATed). In other words, underlay containers are always connected with overlay containers directly, just like they are all overlay containers
3. Traffic between underlay containers keeps the origin "Underlay" attributes. Lower costs and being visible to underlying network

The users of hybridnet can keep both *Overlay* and *Underlay* network inside a Kubernetes cluster without any concern about the connectivity, which brings a more flexible and extensible container network to orchestrate different applications.

As the foundation of hybridnet, we use "Policy Routing" to distribute traffic across the different data planes. The feature of "Policy Routing" is introduced in 2.1 version of linux kernel as a basic part of routing subsystem, which provides strong stability and compatibility. Another two docs about [hybridnet components](/docs/components.md) and [the contrast between hybridnet and other CNI implementations](/docs/other-implementations.md) can be considered as further references.

## Features

- [Unified management APIs](/docs/crd.md) implemented with Kubernetes CRD
- Support IPv4/IPv6 dual stack
- Multiple network fabrics. VXLAN(overlay), VLAN(underlay), BGP(underlay), etc.
- Advanced IPAM. Retaining IP for stateful workloads, topology-aware IP allocation, etc.
- Good compatibility with other networking components (e.g., kube-proxy, cilium)

## How-To-Use

See documents on [wiki](https://github.com/alibaba/hybridnet/wiki).

## Compile and build

Clone the repository to local and `make` can build hybridnet images.

## Contributing

Hybridnet welcome contributions, including bug reports, feature requests and documentation improvements.
If you want to contribute, please start with [CONTRIBUTING.md](CONTRIBUTING.md)

## Contact

For any questions about hybridnet, please reach us via:

- Slack: #general on the [hybridnet slack](https://hybridnetworkspace.slack.com)
- DingTalk: Group No.35109308
- E-mail: private or security issues should be reported via e-mail addresses listed in the [MAINTAINERS](MAINTAINERS) file

## License

[Apache 2.0 License](LICENSE)