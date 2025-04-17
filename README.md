# CloudStorm

CloudStorm is a modular, peer‑to‑peer platform combining:

- **Trinity** (C++ local consensus engine)  
- **CloudStorm** (remote consensus engine, NFT and tokenization services—currently not complete; do not use!)  
- **IPFS** (Tor‑only content distribution relay)  
- **Symbiote** (rippled & Clio — XRPL ledger + dedicated XRPL API server)  

CloudStorm uses Docker Compose for segmented deployments to complement the proof‑of‑state protocol, where each participant node maintains its own file structure and state via blockchain‑inspired strategies.

---

## Features

- **Self‑contained**: local consensus, source‑of‑truth, zero‑trust certificate issuance  
- **Tor‑only IPFS**: full IPFS node over Tor for anonymized communications and peer interactions  
- **Scalable**: automated deployments allow rolling out to many machines simultaneously  
- **Pluggable modules**: segmented responsibility and multi‑level consensus for CaaS (Consensus as a Service)  

---

## System Requirements (entire host)

| Component          | Minimum (Debian 12+ / Ubuntu 20+) | Recommended          |
|--------------------|-----------------------------------|----------------------|
| **CPU (build)**    | 4 cores @ 2 GHz                   | 8 cores @ 3 GHz      |
| **RAM (build)**    | 8 GB                              | 16 GB                |
| **Disk (build)**   | 100 GB free (HDD)                | 100 GB (SSD)         |

---

## Quickstart

```bash
cd /opt
git clone https://github.com/sudocoinxrpl/CloudStorm.git
cd CloudStorm
sudo chmod +x ./preflight.bash
sudo ./preflight.bash
