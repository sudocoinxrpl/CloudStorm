# CloudStorm

CloudStorm is a modular, peer‑to‑peer platform combining:

- **Trinity** (C++ local consensus engine)  
- **CloudStorm** (remote consensus engine, NFT and tokenization services currently not complete do not use!)
- **IPFS** (Tor and IPFS communications relay)  
- **Symbiote** (rippled & Clio** (XRPL ledger + dedicated XRPL API server)  )  

CloudStorm uses Docker Compose for segmented deployments to compliment the proof-of-state protocol where are valid participant nodes are cognizant of their own filestructure through blockchain inspired strategies.

---

## Features

- **Self‑contained**: local consensus, local source of truth, zero trust certificate issuance.
- **Tor‑only IPFS**: Full IPFS node over TOR for anonymized communications and remote peer interactions.
- **Scalable**: an automated deployment scheme means we can easily roll out to as many machines as necessary simultaneously. 
- **Pluggable modules**: Pluggable modules, segmented responsibility, multi level consensus for network CaaS (Consensus as a Service) operations. 

---

## System Requirements (entire host)

| Operating Systems  | Debian 12+ / Ubuntu 20+                   | Recommended              |
|--------------------|-------------------------------------------|--------------------------|
| **CPU (build)**    | 4 cores @ 2 GHz                           | 8 cores @ 3 GHz          |
| **RAM (build)**    | 8 GB                                      | 16 GB                    |
| **Disk (build)**   | 100 GB free (HDD)                          | 100 GB SSD              |

---

## Quickstart

cd /opt
git clone https://github.com/sudocoinxrpl/CloudStorm.git
sudo ./preflight.bash


##troubleshooting

If you reinstall or need to reinstall you should purge the docker volumes first by passing preflight.bash --purge which WILL DESTROY any data held on those containers.
If you do not do this you will see segmentation faults from trinity.

for quicker debugging or module specific debugging, use preflight.bash --rebuild which will reset the containers without touching volumes.
FYI this will never work if any one trinity instance failed and is generally here for adding and removing containers later on.


