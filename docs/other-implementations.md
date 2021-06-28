# Rama vs. Other CNI Implementation

There has already so many of CNI implementations in the open source community, which all exist for a reason. And we also
believe that there is no one perfect for every user. Here we compare Rama with well-known CNI implementations to show
differences.

## Rama vs. Kube-ovn

Kube-ovn is an CNI implementation which integrates the OVN-based Network Virtualization with Kubernetes, offers rich
functions and features, e.g., multi-tenant container network, subnet isolation.

For the overall design, unlike Kube-ovn's offerring rich functions and features, Rama is always designed to be convenient
and widely-adapted. Without multi-tenant network support, Rama takes more efforts to adapt to user's exist underlay network
and always follows the [Kubernetes network model](https://kubernetes.io/docs/concepts/cluster-administration/networking/#the-kubernetes-network-model).
For example, when underlay and overlay network exist in one Rama cluster at the same time (hybrid mode), every underlay
pod will also get a overlay "identity" with the same ip address automatically while communicating with overlay pods, to
ensure the full network connectivity within the cluster.

One of the biggest differences is that Kube-ovn uses OVN/OVS as the dataplane, while Rama uses common networking
abilities of Linux kernel (e.g., policy route, iptables). OVN/OVS is a popular and powerful software-defined
networking (SDN) solution, once you want to know how Kube-OVN works, you should have know OVN/OVS a lot. Rama provides
a less multifunctional but more participatory implementation, to understand Rama, all you need is knowing how to make
networking configurations on a normal Linux distribution, problems of which can always be easily found on StackOverflow
or Google.

Another difference is that subnets of Kube-OVN is associated with namespaces. Namespaced subnets build a strong
relationship between workloads and ip address resource, but it also gets problems when a user
(especially heavy Kubernetes users) don't want to change his original ways or habits of organizing workloads.
Rama provides a total loose coupling relationship between workloads and subnets, which
sometimes makes things more flexible and convenient. By default, a subnet is shared by every workloads, while you can
also create a private subnet which can only be used by specific workloads.

## Rama vs. Calico

[Calico](https://www.projectcalico.org/) is an open-source networking and security solution for Kubernetes with good
performance and security policy.

Both of Rama and Calico provide an end-to-end IP network that interconnects the pod in a scale-out or cloud environment,
by building an *interconnect fabric* to provide the physical networking layer on which Calico or Rama operates.

The main difference between Rama and Calico is the implementation of interconnect fabric in underlay network. Calico
uses [bgp-only interconnect fabrics](https://docs.projectcalico.org/reference/architecture/design/l3-interconnect-fabric#bgp-only-interconnect-fabrics)
while Rama offers a vlan interconnect fabric. This matters when we need the ability of static ip. Once a pod needs to
retain its ip after being recreated, as we never want a pod to be fixed on only one node, a seperate single-ip route
(/32 or /128) will appear in the bgp fabric unavoidably, which requires a very large route table size and might bring
a stability risk into the whole network environment. This will never happen for a vlan fabric, because we always need
just one static route configured manually on the psychical switch for every subnet, which also brings extra operating
costs.

