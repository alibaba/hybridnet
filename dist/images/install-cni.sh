#!/bin/bash

set -u -e

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
