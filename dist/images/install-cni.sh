#!/bin/bash

set -u -e

if [[ -f "/proc/sys/net/bridge/bridge-nf-call-iptables" ]];
    then echo -n 1 > /proc/sys/net/bridge/bridge-nf-call-iptables;
fi

if [[ -f "/proc/sys/net/bridge/bridge-nf-call-ip6tables" ]];
    then echo -n 1 > /proc/sys/net/bridge/bridge-nf-call-ip6tables;
fi

if [[ -f "/proc/sys/net/bridge/bridge-nf-call-arptables" ]];
    then echo -n 1 > /proc/sys/net/bridge/bridge-nf-call-arptables;
fi

if [[ -f "/proc/sys/net/ipv4/ip_forward" ]];
    then echo -n 1 > /proc/sys/net/ipv4/ip_forward;
fi

if [[ -f "/proc/sys/net/ipv6/conf/all/forwarding" ]];
    then echo -n 1 > /proc/sys/net/ipv6/conf/all/forwarding;
fi

if [[ -f "/proc/sys/net/ipv4/conf/all/rp_filter" ]];
    then echo -n 0 > /proc/sys/net/ipv4/conf/all/rp_filter;
fi

if [[ -f "/proc/sys/net/ipv4/conf/all/arp_filter" ]];
    then echo -n 0 > /proc/sys/net/ipv4/conf/all/arp_filter;
fi

CNI_BIN_SRC=/hybridnet/hybridnet
CNI_BIN_DST=/opt/cni/bin/hybridnet

COMMUNITY_CNI_PLUGINS_SRC_DIR=/cni-plugins
COMMUNITY_CNI_PLUGINS_DST_DIR=/opt/cni/bin

PLUGINS=${NEEDED_COMMUNITY_CNI_PLUGINS-"loopback"}
for plugin in ${PLUGINS//,/ }
do
  cp -f $COMMUNITY_CNI_PLUGINS_SRC_DIR/"$plugin" $COMMUNITY_CNI_PLUGINS_DST_DIR/"$plugin"
done

cp -f $CNI_BIN_SRC $CNI_BIN_DST

# clean the out-of-date configuration
rm -rf /etc/cni/net.d/*-hybridnet.conflist

CNI_CONF_DST=/etc/cni/net.d/${CNI_CONF_NAME-"06-hybridnet.conflist"}
cp -f "${CNI_CONF_SRC-"/hybridnet/00-hybridnet.conflist"}" "$CNI_CONF_DST"
