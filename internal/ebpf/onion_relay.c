#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ipv6.h>
#include <linux/tcp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

/* VX6 Protocol Constants */
#define VX6_MAGIC 0x31365856 // "VX61"
#define KIND_ONION 5

struct onion_packet_header {
    __u32 magic;
    __u8 kind;
    __u32 payload_len;
    __u32 hop_count;
    struct in6_addr hops[5];
} __attribute__((packed));

SEC("xdp")
int xdp_onion_relay(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) return XDP_PASS;

    if (eth->h_proto != bpf_htons(ETH_P_IPV6)) return XDP_PASS;

    struct ipv6hdr *ip6 = (void *)(eth + 1);
    if ((void *)(ip6 + 1) > data_end) return XDP_PASS;

    if (ip6->nexthdr != IPPROTO_TCP) return XDP_PASS;

    struct tcphdr *tcp = (void *)(ip6 + 1);
    if ((void *)(tcp + 1) > data_end) return XDP_PASS;

    // Inspect TCP Payload for VX6 Header
    struct onion_packet_header *vx6 = (void *)(tcp + 1);
    if ((void *)(vx6 + 1) > data_end) return XDP_PASS;

    if (vx6->magic == bpf_htonl(VX6_MAGIC) && vx6->kind == KIND_ONION) {
        // FAST PATH: If hop_count < 4, we are a relay.
        if (vx6->hop_count < 4) {
            vx6->hop_count++;
            
            // Swap Destination IPv6 to next hop
            ip6->daddr = vx6->hops[vx6->hop_count];
            
            // Swap MAC addresses for reflection (Source -> Next Hop)
            __u8 tmp_mac[6];
            __builtin_memcpy(tmp_mac, eth->h_dest, 6);
            __builtin_memcpy(eth->h_dest, eth->h_source, 6);
            __builtin_memcpy(eth->h_source, tmp_mac, 6);

            return XDP_TX; // Reflect packet back out (Zero-copy forward)
        }
    }

    return XDP_PASS; // Send to Go app for final processing or exit node logic
}

char _license[] SEC("license") = "GPL";
