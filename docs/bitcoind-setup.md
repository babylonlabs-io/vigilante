# Bitcoin Node Setup

This document guides operators through setting up a Bitcoin node for use with Vigilante, including installation, configuration, and running the bitcoinds.

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Install Bitcoin Core](#2-install-bitcoin-core)
3. [Setup bitcoind](#3-setup-bitcoind)
4. [Start bitcoind](#4-start-bitcoind)

## 1. Prerequisites

### 1.1 Hardware Requirements

Ensure your system meets the following minimum requirements:

* **CPU:** At least 4 vCPUs
* **RAM:** At least 32 GB
* **Storage:** 1 TB SSD
* **Network:** Stable internet connection

## 2. Install Bitcoin Core

Download and install the Bitcoin binaries for your operating system from the official [Bitcoin Core registry](https://bitcoincore.org/bin/bitcoin-core-26.0/).

Verify installation by checking the version:

```shell
bitcoind --version
```

## 3. Setup bitcoind

### 3.1 Generate rpcauth config

Always use `rpcauth` instead of storing the RPC password (`rpcpassword`) in plain
text within the configuration file.
This approach prevents direct exposure of credentials.

You may refer to an [online tool](https://jlopp.github.io/bitcoin-core-rpc-auth-generator/)
for generating the hash that will be used in the `bitcoin.conf` config.
The final configuration should be structured as follows:

```bash
rpcauth=your_rpc_user:generated_salted_hash
```

### 3.2 Configure bitcoind

Bitcoind is configured through a main configuration file named bitcoin.conf.

Depending on the operating system, the configuration file should be placed
under the corresponding path:

* MacOS: `/Users/<username>/Library/Application Support/Bitcoin`
* Linux: `/home/<username>/.bitcoin`
* Windows: `C:\Users\<username>\AppData\Roaming\Bitcoin`

Use the following configuration for BTC Signet:

```bash
# Accept command line and JSON-RPC commands
server=1

# Enable transaction indexing
txindex=1

# RPC server settings
rpcauth=your_rpc_user:generated_salted_hash
rpcbind=0.0.0.0
rpcallowip=0.0.0.0/0

# ZMQ notification options
zmqpubsequence=tcp://0.0.0.0:29000
zmqpubrawblock=tcp://0.0.0.0:29001
zmqpubrawtx=tcp://0.0.0.0:29002

# Fallback fee for transactions
fallbackfee=0.00001

[signet]
signet=1
rpcport=38332
```

Configuration parameters explained:

* `server`: Enables JSON-RPC interface
* `txindex`: Maintains full transaction index
* `rpcauth`: Defines authentication credentials for the RPC interface
* `signet`: Enables operation on the Signet network instead of Mainnet.

## 4. Start bitcoind

Start the daemon with:

```bash
bitcoind
```

You can check the status of bitcoind by running:

```bash
bitcoin-cli getblockchaininfo
```

You should see information about the blockchain, indicating that bitcoind is running correctly.
