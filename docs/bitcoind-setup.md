# Bitcoin Node Setup

This document guides operators through setting up a Bitcoin node for use with Vigilante, including installation, configuration, and running the bitcoinds.

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Install Bitcoin Core](#2-install-bitcoin-core)
3. [Setup bitcoind](#3-setup-bitcoind)
4. [Start bitcoind](#4-start-bitcoind)
5. [Setup Electrum Server](#5-setup-electrum-server)

## 1. Prerequisites

### 1.1 Hardware Requirements

Ensure your system meets the following minimum requirements:

* **CPU:** At least 4 vCPUs
* **RAM:** At least 32 GB
* **Storage:** 2 TB SSD
* **Network:** Stable internet connection

## 2. Install Bitcoin Core

Download and install the Bitcoin v26.0 binary for your operating system from the official
[Bitcoin Core registry](https://bitcoincore.org/bin/bitcoin-core-26.0/).

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

Bitcoind is configured through a main configuration file named `bitcoin.conf`.

Depending on the operating system, the configuration file should be placed under
the corresponding path:

* MacOS: `/Users/<username>/Library/Application Support/Bitcoin`
* Linux: `/home/<username>/.bitcoin`
* Windows: `C:\Users\<username>\AppData\Roaming\Bitcoin`

Use the following configuration:

```bash
# Set network mode (uncomment the appropriate network)
# mainnet=1
signet=1

# Accept command line and JSON-RPC commands
server=1

# Enable transaction indexing
txindex=1

# RPC server settings
rpcauth=your_rpc_user:generated_salted_hash

# ZMQ notification options
zmqpubsequence=tcp://0.0.0.0:29000
zmqpubrawblock=tcp://0.0.0.0:29001
zmqpubrawtx=tcp://0.0.0.0:29002

# Fallback fee for transactions
fallbackfee=0.00001

# Network-specific settings
[signet]
rpcport=38322
rpcbind=0.0.0.0
rpcallowip=0.0.0.0/0

[mainnet]
rpcport=8332
rpcbind=127.0.0.1
rpcallowip=127.0.0.1/32
```

Configuration parameters explained:

* `server`: Enables JSON-RPC interface
* `txindex`: Maintains full transaction index
* `rpcauth`: Defines authentication credentials for the RPC interface
* `rpcbind`: Binds RPC server to a specific address (must be in the correct
  network section)
* `rpcallowip`: Allows specific IP ranges to connect to the RPC interface
* `signet` or `mainnet`: Enables the appropriate network

## 4. Start bitcoind

Start the daemon with:

```bash
bitcoind
```

You can check the status of bitcoind by running:

```bash
bitcoin-cli getblockchaininfo
```

To check if your node is still syncing:

* Check the `initialblockdownload` status:

```bash
bitcoin-cli getblockchaininfo | grep initialblockdownload
```

If this returns `"initialblockdownload": true`, the node is still syncing.

* Compare your current block height with external sources:

```bash
bitcoin-cli getblockchaininfo | grep blocks
bitcoin-cli getpeerinfo | grep startingheight
```

If your block height is significantly lower than the peer `startingheight`
values, the node is still catching up.

You can also compare with public blockchain explorers like
[mempool.space](https://mempool.space) to verify the latest block height.

## 5. Setup Electrum Server

We will use [Electrs](https://github.com/Blockstream/electrs), a more efficient
Electrum server implementation.

Electrs should be operated on the same node with bitcoind, so that it can
directly access the bitcoind filesystem and achieve optimal performance.

### 5.1 Install Dependencies

Ensure you have the necessary dependencies installed:

```bash
sudo apt update
sudo apt install -y cargo clang cmake build-essential
```

### 5.2 Build Electrs Locally

Clone the repository and build Electrs:

```bash
git clone https://github.com/Blockstream/electrs.git
cd electrs
git checkout e11cd561a197f8c0e4839721b4e1877b2a41053f
cargo build --release
```

### 5.3 Start Electrs

Create a directory for Electrs data:

```bash
mkdir -p ~/.electrs
```

Use the following command to start Electrs with the appropriate configuration:

```bash
electrs --network signet \
        --electrum-rpc-addr "0.0.0.0:5000" \
        --http-addr "0.0.0.0:3000" \
        --db-dir ~/.electrs/db/ \
        --daemon-rpc-addr "localhost:38332" \
        --daemon-dir ~/.bitcoin \
        --monitoring-addr "0.0.0.0:9334" \
        --address-search \
        --cors '*' \
        --utxos-limit 100000 \
        -v \
        --timestamp \
        --electrum-rpc-logging full
```

Configuration parameters explained:

* `--network signet`: Uses Signet as the network (change to `mainnet` if needed)
* `--cookie`: Authentication credentials for connecting to Bitcoin Core
* `--db-dir`: Directory where Electrs stores its database (`~/.electrs/db/`)
* `--daemon-dir`: Directory where Bitcoin Core data is stored (`~/.bitcoin`)
* `--electrum-rpc-addr`: Port for Electrum protocol
* `--http-addr`: Esplora HTTP port
* `--daemon-rpc-addr`: RPC address to communicate with bitcoind
* `--monitoring-addr`: Prometheus monitoring address

You should see logs indicating successful startup:

```log
Config { log: StdErrLog { verbosity: Warn, quiet: false, show_level: true, timestamp: Millisecond, modules: [], writer: "stderr", color_choice: Never, show_module_names: false }, network_type: Signet, db_path: "/electrs/.electrs/db/signet", daemon_dir: "/bitcoin/.bitcoin/signet", blocks_dir: "/bitcoin/.bitcoin/signet/blocks", daemon_rpc_addr: 127.0.0.1:38332, cookie: Some("K78L47aCp6NrcLnG0sTD8k5oaNZuwK1m:YIr0Y7gMHPofvBDmZYmu2Cm0gR7OGz5x"), electrum_rpc_addr: 0.0.0.0:60001, http_addr: 0.0.0.0:3003, http_socket_file: None, monitoring_addr: 0.0.0.0:54224, jsonrpc_import: false, light_mode: false, address_search: true, index_unspendables: false, cors: Some("*"), precache_scripts: None, utxos_limit: 50000, electrum_txs_limit: 500, electrum_banner: "Welcome to electrs-esplora 0.4.1", electrum_rpc_logging: Some(Full) }
```

On the first boot, the indexer may take several hours to complete the initial sync.
