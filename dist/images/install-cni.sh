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

exit_with_error(){
  echo $1
  exit 1
}

CNI_BIN_SRC=/rama/rama
CNI_BIN_DST=/opt/cni/bin/rama

CNI_CONF_SRC=/rama/00-rama.conflist
CNI_CONF_DST=/etc/cni/net.d/00-rama.conflist

LOOPBACK_BIN_SRC=/cni-plugins/loopback
LOOPBACK_BIN_DST=/opt/cni/bin/loopback

BANDWIDTH_BIN_SRC=/cni-plugins/bandwidth
BANDWIDTH_BIN_DST=/opt/cni/bin/bandwidth

yes | cp -f $LOOPBACK_BIN_SRC $LOOPBACK_BIN_DST || exit_with_error "Failed to copy $LOOPBACK_BIN_SRC to $LOOPBACK_BIN_DST"
yes | cp -f $BANDWIDTH_BIN_SRC $BANDWIDTH_BIN_DST || exit_with_error "Failed to copy $BANDWIDTH_BIN_SRC to $BANDWIDTH_BIN_DST"
yes | cp -f $CNI_BIN_SRC $CNI_BIN_DST || exit_with_error "Failed to copy $CNI_BIN_SRC to $CNI_BIN_DST"
yes | cp -f $CNI_CONF_SRC $CNI_CONF_DST || exit_with_error "Failed to copy $CNI_CONF_SRC to $CNI_CONF_DST"
