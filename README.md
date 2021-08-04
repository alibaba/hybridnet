# Rama

[![Go Report Card](https://goreportcard.com/badge/github.com/oecp/rama)](https://goreportcard.com/report/github.com/oecp/rama)
[![Github All Releases](https://img.shields.io/docker/pulls/ramanetworking/rama.svg)](https://hub.docker.com/r/ramanetworking/rama/tags)
[![codecov](https://codecov.io/gh/oecp/rama/branch/main/graphs/badge.svg)](https://codecov.io/gh/oecp/rama)

## What is Rama?

Rama is an open source container networking solution, integrated with Kubernetes and used officially by following well-known PaaS platforms,

- OECP of Alibaba Cloud
- SOFAStack of Ant Financial Co.

Rama focus on large-scale, user-friendly and heterogeneous infrastructure, now hundreds of clusters are running on rama all over world.

## Features

- Flexible network models: three-level, **Network, Subnet and IPInstance**, all implemented in CRD
- DualStack: three modes optional, IPv4Only, IPv6Only and DualStack
- Hybrid network fabric: support overlay and underlay pods at same time
- Advanced IPAM: Network/Subnet/IPInstance assignment; stateful workloads IP retain
- Kube-proxy friendly: working well with iptables-mode kube-proxy
- ARM support: run on x86_64 and arm64 architectures

## Contributing

Rama welcome contributions, including bug reports, feature requests and documentation improvements.
If you want to contribute, please start with [CONTRIBUTING.md](CONTRIBUTING.md)

## Contact

For any questions about rama, please reach us via:

- Slack: #general on the [rama slack](rama-qh63190.slack.com)
- DingTalk: Group No.35109308
- E-mail: private or security issues should be reported via e-mail addresses listed in the [MAINTAINERS](MAINTAINERS) file

## License

[Apache 2.0 License](LICENSE)