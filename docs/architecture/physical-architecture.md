# Physical Architecture Diagram

```mermaid
graph TB
    subgraph "Home Network 192.168.1.0/24"
        Router[Router/Gateway<br/>192.168.1.1]
        
        subgraph "Physical Servers"
            Ocean[🐋 Ocean<br/>Ubuntu + Docker + ZFS<br/>192.168.1.143<br/>NVIDIA P2000 GPU]
            Node006[🏗️ Node006<br/>Proxmox Host<br/>192.168.1.106]
        end
        
        subgraph "VMs on Node006"
            DNS01[🌐 dns01<br/>DNS Server<br/>192.168.1.2]
            Pihole[🕳️ pihole<br/>Ad Blocker<br/>192.168.1.9]
            Gitlab[🦊 gitlab<br/>Git Server<br/>192.168.1.5]
        end
    end
    
    Internet[🌍 Internet] --> Router
    Router --> Ocean
    Router --> Node006
    Router --> DNS01
    Router --> Pihole
    Router --> Gitlab
    
    Node006 -.-> DNS01
    Node006 -.-> Pihole
    Node006 -.-> Gitlab
    
    style Ocean fill:#e1f5fe
    style Node006 fill:#f3e5f5
    style DNS01 fill:#e8f5e8
    style Pihole fill:#fff3e0
    style Gitlab fill:#fce4ec
```
