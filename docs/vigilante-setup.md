# Vigilante Setup

This document guides operators through the complete lifecycle of
running a Vigilante, including:

* Installing and configuring the Vigilante
* Setting up the directory and configuration
* Managing the Babylon keyring
* Running Vigilante daemon

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Install Vigilante](#2-install-vigilante)
3. [Set Up Vigilante](#3-set-up-vigilante)
    1. [Initialize Directory](#31-initialize-directory)
    2. [Configure Vigilante](#32-configure-vigilante)
4. [Set Up Babylon Keyring Directory](#4-set-up-babylon-keyring-directory)
5. [Start the Vigilante Daemon](#5-start-the-vigilante-daemon)

## 1. Prerequisites

To successfully complete this guide, you will need:

### 1.1 Hardware Requirements

Ensure your system meets the following minimum requirements:

* **CPU:** 4 vCPUs
* **RAM:** 8 GB
* **Storage:** 50 GB SSD/NVME
* **Network:** Stable internet connection

### 1.2 Software Requirements

* A connection to a Babylon node. To run your own node, refer to the
[Babylon Node Setup Guide](https://github.com/babylonlabs-io/networks/blob/main/bbn-test-5/babylon-node/README.md).
* A connection to a Bitcoin node. To run your own node, refer to the
[Bitcoin Node Setup Guide](./bitcoind-setup.md).

## 2. Install Vigilante

If you haven't already, download [Golang 1.23](https://go.dev/dl).

Once installed, run:

```shell
go version
```

If you have not yet cloned the repository, run:

```shell
git clone git@github.com:babylonlabs-io/vigilante.git
cd vigilante
git checkout <tag>
```

Run the following command to build the binaries and
install them to your `$GOPATH/bin` directory:

```shell
make install
```

This command will:

* Build and compile all Go packages
* Install `vigilante` binary to `$GOPATH/bin`
* Make commands globally accessible from your terminal

If your shell cannot find the installed binary, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Use the following command to add it to your profile
depending on your shell.

```shell
export PATH=$HOME/go/bin:$PATH
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
```

## 3. Set Up Vigilante

### 3.1 Initialize directory

Next, create the necessary directory and copy the sample
configuration file to initialize the home directory.

```shell
mkdir -p ~/config

cp ~/vigilante/sample-vigilante.yml ~/config/vigilante.yml
```

### 3.2. Configure Vigilante

Edit the `vigilante.yml` file in your vigilante home directory
with the following parameters:

```yaml
btc:
  endpoint: localhost:38332
  tx-fee-min: 500
  tx-fee-max: 20000
  zmq-seq-endpoint: tcp://localhost:29000
  zmq-block-endpoint: tcp://localhost:29001
  zmq-tx-endpoint: tcp://localhost:29002
  wallet-name: default
  wallet-password: walletpass
  username: rpuser
  password: rpcpassword

babylon:
  key: key
  chain-id: bbn-test-5
  rpc-addr: http://localhost:26657
  grpc-addr: https://localhost:9090
  account-prefix: bbn
  keyring-backend: test
  gas-adjustment: 2
  gas-prices: 0.002ubbn
  key-directory: /home/vigilante/.babylond

submitter:
  netparams: signet
  buffer-size: 100
  resubmit-fee-multiplier: 1.3
  polling-interval-seconds: 60
  resend-interval-seconds: 600
  dbconfig:
    dbpath: /home/vigilante/data
    dbfilename: submitter.db

reporter:
  netparams: signet
  btc_cache_size: 10000
  max_headers_in_msg: 100

monitor:
  checkpoint-buffer-size: 1000
  btc-block-buffer-size: 1000
  btc-cache-size: 1000
  btc-confirmation-depth: 6
  liveness-check-interval-seconds: 100
  max-live-btc-heights: 200
  enable-liveness-checker: true
  dbconfig:
    dbpath: /home/vigilante/data
    dbfilename: monitor.db

btcstaking-tracker:
  check-delegations-interval: 1m
  delegations-batch-size: 100
  check-if-delegation-active-interval: 5m
  retry-submit-unbonding-interval: 1m
  max-jitter-interval: 30s
  btcnetparams: signet
  max-slashing-concurrency: 20
```

Configuration parameters explained:

#### 3.2.1 Bitcoin Configuration

* `endpoint`: Address where your Bitcoin node is running
* `tx-fee-min`: Minimum transaction fee in satoshis
* `tx-fee-max`: Maximum transaction fee in satoshis
* `zmq-seq-endpoint`: Your Bitcoin node's ZMQ sequence endpoint
* `zmq-block-endpoint`: Your Bitcoin node's ZMQ block endpoint
* `zmq-tx-endpoint`: Your Bitcoin node's ZMQ tx endpoint
* `wallet-name`: Name of the Bitcoin wallet
* `wallet-password`: Password for the Bitcoin wallet
* `username`: Username for RPC authentication
* `password`: Password for RPC authentication

#### 3.2.2 Babylon Configuration

* `rpc-addr`: Your Babylon node's RPC endpoint
* `grpc-addr`: Your Babylon node's GRPC endpoint
* `key`: Your Babylon key
* `chain-id`: Chain ID of the Babylon network
* `gas-adjustment`: Gas adjustment factor
* `gas-prices`: Gas price for transactions
* `key-directory`: Directory where the keyring is stored. **Before proceeding,
ensure the Babylon keyring is set up. Follow
[Step 4](#4-set-up-babylon-keyring-directory) for detailed instructions.**

#### 3.2.3 Submitter Configuration

* `netparams`: Bitcoin network parameters
* `buffer-size`: Size of the transaction buffer
* `resubmit-fee-multiplier`: Multiplier for resubmission fees
* `polling-interval-seconds`: Interval for polling transactions
* `resend-interval-seconds`: Interval for resending transactions
* `dbpath`: Path where the submitter database is stored
* `dbfilename`: Name of the submitter database file

#### 3.2.4 Reporter Configuration

* `netparams`: Bitcoin network parameters
* `max_headers_in_msg`: Maximum number of headers in a message

#### 3.2.5 Monitor Configuration

* `checkpoint-buffer-size`: Buffer size for checkpoints
* `btc-block-buffer-size`: Buffer size for Bitcoin blocks
* `btc-cache-size`: Bitcoin cache size
* `btc-confirmation-depth`: Depth for Bitcoin confirmations
* `liveness-check-interval-seconds`: Interval for liveness checks
* `max-live-btc-heights`: Maximum number of live Bitcoin heights to track
* `enable-liveness-checker`: Whether to enable the liveness checker
* `dbpath`: Path where the monitor database is stored
* `dbfilename`: Name of the monitor database file

#### 3.2.6 BTC Staking Tracker

* `check-delegations-interval`: Interval for checking delegations
* `delegations-batch-size`: Batch size for delegations
* `check-if-delegation-active-interval`: Interval for checking active delegations
* `retry-submit-unbonding-interval`: Interval for retrying unbonding submissions
* `btcnetparams`: Bitcoin network parameters
* `max-slashing-concurrency`: Maximum concurrency for slashing

## 4. Set Up Babylon Keyring Directory

This section explains the process of setting up the Babylon keyring.
Operators must create an Babylon keyring before starting
the `Vigilante Submitter` and `Vigilante BTC Staking Tracker` daemon.

We will be using the Babylon Binary for the key generation. To install the binary,
please refer to the [Babylon Binary Installation](https://github.com/babylonlabs-io/networks/blob/main/bbn-test-5/babylon-node/README.md#1-install-babylon-binary).

### 4.1 Initialize Babylon home dir

```shell
babylond init <moniker> --chain-id bbn-test-5 --home <path>
```

Parameters:

* `<moniker>`: A unique identifier for your node for example `node0`
* `--chain-id`: The chain ID of the Babylon chain you connect to
* `--home`: *optional* flag that specifies the directory where your
   node files will be stored, for example `--home ./nodeDir`
   The default home directory for your Babylon node is:
  * Linux/Mac: `~/.babylond/`
  * Windows: `%USERPROFILE%\.babylond\`

### 4.2 Generate Babylon account keyring

```shell
babylond keys add <name> --home <path> --keyring-backend test
```

Parameters:

* `<name>` specifies a unique identifier for the key.
* `--home` specifies the directory where your node files will be stored
(e.g. `--home ./nodeDir`)

Since this key is accessed by an automated daemon process, it must be stored
unencrypted on disk and associated with the `test` keyring backend.

The execution result displays the address of the newly generated key and its
public key. Following is a sample output for the command:

```shell
*address: bbn1kvajzzn6gtfn2x6ujfvd6q54etzxnqg7pkylk9
  name: <name>
  pubkey: '{"@type":"/cosmos.crypto.secp256k1.PubKey",
           key: "Ayau+8f945c1iQp9tfTVaCT5lzhD8n4MRkZNqpoL6Tpo"}'
  type: local
```

## 5. Start the vigilante daemon

You can start the Vigilante daemon using one of the available commands:

```shell
vigilante [command] --config ~/config/vigilante.yml
```

You should see logs indicating successful startup:

```shell
info Successfully connected to bitcoind {"module": "btcclient_wallet"}
```

Available commands include:

* `bstracker`: To run the Vigilante BTC Staking Tracker
* `monitor`: To run the Vigilante Monitor
* `reporter`: To run Vigilante reporter
* `submitter`: To run Vigilante submitter

Available flags include:

* `--config`: Specifies the path to configuration file
