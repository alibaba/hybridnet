# Hybridnet

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/hybridnet)](https://goreportcard.com/report/github.com/alibaba/hybridnet)
[![Github All Releases](https://img.shields.io/docker/pulls/hybridnetdev/hybridnet.svg)](https://hub.docker.com/r/hybridnetdev/hybridnet/tags)
[![Version](https://img.shields.io/github/v/release/alibaba/hybridnet)](https://github.com/alibaba/hybridnet/releases)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/hybridnet)](https://artifacthub.io/packages/search?repo=hybridnet)
[![License](https://img.shields.io/github/license/alibaba/hybridnet)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![codecov](https://codecov.io/gh/alibaba/hybridnet/branch/main/graphs/badge.svg)](https://codecov.io/gh/alibaba/hybridnet)
![workflow check](https://github.com/alibaba/hybridnet/actions/workflows/check.yml/badge.svg)
![workflow build](https://github.com/alibaba/hybridnet/actions/workflows/build.yml/badge.svg)

## What is Hybridnet?

Hybridnet is an open source container networking solution designed for hybrid clouds, integrated with Kubernetes and used officially by following well-known PaaS platforms,

- ACK Distro of Alibaba Cloud
- AECP of Alibaba Cloud
- SOFAStack of Ant Financial Co.

Hybridnet focus on productive large-scale, user friendliness and heterogeneous infrastructure, now hundreds of clusters are running on hybridnet all over world.

## Features

- Flexible network models: three-level, **Network, Subnet and IPInstance**, all implemented in CRD
- DualStack: three modes optional, IPv4Only, IPv6Only and DualStack
- Hybrid network fabric: support overlay and underlay pods at same time on node level
- Cluster mesh: support network connectivity among different clusters
- Advanced IPAM: Network/Subnet/IPInstance assignment; stateful workloads IP retain
- Integration friendly: working well with other networking components (e.g., kube-proxy, cilium)
- ARM support: run on x86_64 and arm64 architectures

## How-To-Use

See documents on [wiki](https://github.com/alibaba/hybridnet/wiki).

## Contributing

Hybridnet welcome contributions, including bug reports, feature requests and documentation improvements.
If you want to contribute, please start with [CONTRIBUTING.md](CONTRIBUTING.md)

## Contact

For any questions about hybridnet, please reach us via:

- Slack: #general on the [hybridnet slack](hybridnetworkspace.slack.com)
- DingTalk: Group No.35109308
- E-mail: private or security issues should be reported via e-mail addresses listed in the [MAINTAINERS](MAINTAINERS) file

## License

[Apache 2.0 License](LICENSE)