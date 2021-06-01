# Rama

Rama is an open source container networking solution for Kubernetes, using standard Linux networking tools to provide a simple but powerful network fabric.

Rama uses a [model]() of **Network/Subnet/IPInstance** to describe a Kubernetes container network, which includes two kinds of Network type now:
- Underlay, based on a classical data link layer switching network and is adapter to multiple vlan/subnet/asw environment
- Overlay, a vxlan network without fixed ip blocks for each node, provides flexible IPAM strategy just the same as an Underlay network  

Rama also supports creating Overlay and Underlay network in one cluster at the same time. Using such a *Hybrid* network fabric, you can build a much more powerful container network.

## Features
Besides the Hybrid network fabric, Rama also supports: 

- IPv4/IPv6 dual stack
- Flexible IPAM strategy: keeping Statefulset Pod's ip fixed, assigning Pod to Network/Subnet, etc.
- Manage network resource by kubectl commands with Network/Subnet CR

## Contributing

Rama is an open source project, and welcomes your contribution, be it through code, a bug report, a feature request, or user
feedback.

See [CONTRIBUTING](CONTRIBUTING.md) document will get you started on submitting changes to the project.

## License

Rama is open source, with most code and documentation available under the Apache 2.0 license (see the [LICENSE](LICENSE))