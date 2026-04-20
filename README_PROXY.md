# VX6 Proxy Mode: 5-Hop Onion Routing

This branch contains the high-performance anonymity and relay layer for VX6.

### **The Architecture**
Data is wrapped in 5 layers of routing information. Every node in the chain only knows the previous and next hops.

```mermaid
graph LR
    User[Your PC] -- Layer 5 --> N1[Relay 1]
    N1 -- Layer 4 --> N2[Relay 2]
    N2 -- Layer 3 --> N3[Relay 3]
    N3 -- Layer 2 --> N4[Relay 4]
    N4 -- Layer 1 --> N5[Exit Node]
    N5 -- Decrypted --> Dst[Actual Destination]
```

### **Requirements**
1.  **Registry Cache**: You must have at least 5 peers in your `vx6 list`.
2.  **Linux Kernel**: eBPF/XDP requires Linux 5.4+.
3.  **Packages**: `iproute2`, `clang`, `llvm`.

### **How to Use (Draft Implementation)**

Currently, you can manually trigger an onion-routed test.

**1. On your machine:**
Define the chain and destination:
```bash
# This wraps a message in 5 layers and sends it through the chain
./vx6 debug onion-test --hops "addr1,addr2,addr3,addr4,addr5" --dst "127.0.0.1:80"
```

### **Benefits of eBPF in Proxy Mode**
*   **Kernel Fast-Path**: Packets are relayed at Layer 2/3. The CPU never context-switches to the Go application for relaying.
*   **Wire Speed**: A relay node can handle millions of packets per second with negligible CPU usage.
*   **Security**: The XDP program drops malformed packets before they reach the OS networking stack.

### **Building with eBPF**
Ensure you have `clang` and `llvm` installed, then run:
```bash
make
```
This generates `vx6` (the binary) and `onion_relay.o` (the kernel bytecode).
