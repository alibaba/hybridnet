# hybridnet

hybridnet is an open source container networking solution designed for hybrid clouds.

![Version: 0.1.1](https://img.shields.io/badge/Version-0.1.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.4.2](https://img.shields.io/badge/AppVersion-0.4.2-informational?style=flat-square)

## Prerequisites

- Kubernetes v1.16+

## Get Repo Info

```shell
helm repo add hybridnet https://alibaba.github.io/hybridnet/
helm repo update
```

## Install Chart

**Important:** only helm3 is supported

```shell
helm install hybridnet hybridnet/hybridnet -n kube-system
```
The command deploys hybridnet on the Kubernetes cluster in the default configuration.

_See [configuration](#configuration) below._

_See [helm install](https://helm.sh/docs/helm/helm_install/) for command documentation._

## Upgrade Chart

```shell
helm upgrade hybridnet hybridnet/hybridnet -n kube-system
```

_See [helm upgrade](https://helm.sh/docs/helm/helm_upgrade/) for command documentation._

## Configuration

To see all configurable options with detailed comments, visit the chart's [values.yaml](./values.yaml),
or run these configuration commands:

```shell
helm show values hybridnet/hybridnet
```

### Change default network type

After deploying hybridnet with the default configuration, you can change the default network type any 
time with these commands:

```shell
# Change default network type to Underlay
helm upgrade hybridnet hybridnet/hybridnet -n kube-system --set advancedSettings.defualtNetworkType=Underlay
```

Of course, if you want to change your container network to use Underlay as default network type, you should
apply some Underlay _Network/Subnet_ CR resources firstly.