#!/bin/sh

cleanup_felix() {
    # Set FORWARD action to ACCEPT so outgoing packets can go through POSTROUTING chains.
    echo "Setting default FORWARD action to ACCEPT..."
    iptables -P FORWARD ACCEPT

    # Make sure ip_forward sysctl is set to allow ip forwarding.
    sysctl -w net.ipv4.ip_forward=1

    echo "Starting the flush Calico policy rules..."
    echo "Make sure calico-node DaemonSet is stopped before this gets executed."

    echo "Flushing all the calico iptables chains in the nat table..."
    iptables-save -t nat | grep -oP '(?<!^:)cali-[^ ]+' | while read line; do iptables -t nat -F "$line"; done

    echo "Flushing all the calico iptables chains in the raw table..."
    iptables-save -t raw | grep -oP '(?<!^:)cali-[^ ]+' | while read line; do iptables -t raw -F "$line"; done

    echo "Flushing all the calico iptables chains in the mangle table..."
    iptables-save -t mangle | grep -oP '(?<!^:)cali-[^ ]+' | while read line; do iptables -t mangle -F "$line"; done

    echo "Flushing all the calico iptables chains in the filter table..."
    iptables-save -t filter | grep -oP '(?<!^:)cali-[^ ]+' | while read line; do iptables -t filter -F "$line"; done

    echo "Cleaning up calico rules from the nat table..."
    iptables-save -t nat | grep -e '--comment "cali:' | cut -c 3- | sed 's/^ *//;s/ *$//' | xargs -l1 iptables -t nat -D

    echo "Cleaning up calico rules from the raw table..."
    iptables-save -t raw | grep -e '--comment "cali:' | cut -c 3- | sed 's/^ *//;s/ *$//' | xargs -l1 iptables -t raw -D

    echo "Cleaning up calico rules from the mangle table..."
    iptables-save -t mangle | grep -e '--comment "cali:' | cut -c 3- | sed 's/^ *//;s/ *$//' | xargs -l1 iptables -t mangle -D

    echo "Cleaning up calico rules from the filter table..."
    iptables-save -t filter | grep -e '--comment "cali:' | cut -c 3- | sed 's/^ *//;s/ *$//' | xargs -l1 iptables -t filter -D

    echo "Flushing all the calico ip6tables chains in the nat table..."
    ip6tables-save -t nat | grep -oP '(?<!^:)cali-[^ ]+' | while read line; do ip6tables -t nat -F "$line"; done

    echo "Flushing all the calico ip6tables chains in the raw table..."
    ip6tables-save -t raw | grep -oP '(?<!^:)cali-[^ ]+' | while read line; do ip6tables -t raw -F "$line"; done

    echo "Flushing all the calico ip6tables chains in the mangle table..."
    ip6tables-save -t mangle | grep -oP '(?<!^:)cali-[^ ]+' | while read line; do ip6tables -t mangle -F "$line"; done

    echo "Flushing all the calico ip6tables chains in the filter table..."
    ip6tables-save -t filter | grep -oP '(?<!^:)cali-[^ ]+' | while read line; do ip6tables -t filter -F "$line"; done

    echo "Cleaning up calico rules from the nat table..."
    ip6tables-save -t nat | grep -e '--comment "cali:' | cut -c 3- | sed 's/^ *//;s/ *$//' | xargs -l1 ip6tables -t nat -D

    echo "Cleaning up calico rules from the raw table..."
    ip6tables-save -t raw | grep -e '--comment "cali:' | cut -c 3- | sed 's/^ *//;s/ *$//' | xargs -l1 ip6tables -t raw -D

    echo "Cleaning up calico rules from the mangle table..."
    ip6tables-save -t mangle | grep -e '--comment "cali:' | cut -c 3- | sed 's/^ *//;s/ *$//' | xargs -l1 ip6tables -t mangle -D

    echo "Cleaning up calico rules from the filter table..."
    ip6tables-save -t filter | grep -e '--comment "cali:' | cut -c 3- | sed 's/^ *//;s/ *$//' | xargs -l1 ip6tables -t filter -D
}

cleanup_felix
