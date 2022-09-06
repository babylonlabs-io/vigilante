# vigilante

Vigilante program for Babylon

## Requirements

- Go 1.18

## Building

```bash
$ go build ./cmd/main.go
```

Note that Vigilante depends on https://github.com/babylonchain/babylon, which is still a private repository.
In order to allow Go to retrieve private dependencies, one needs to enforce Git to use SSH (rather than HTTPS) for authentication, by adding the following lines to your `~/.gitconfig`:

```
[url "ssh://git@github.com/"]
	insteadOf = https://github.com/
```

See https://go.dev/doc/faq#git_https for more information.

## Running the vigilante locally

1. Launch a Bitcoin node

```bash
$ btcd --simnet --rpclisten 127.0.0.1:18554 --rpcuser user --rpcpass pass --miningaddr SQqHYFTSPh8WAyJvzbAC8hoLbF12UVsE5s
```

2. Launch a Babylon node following Babylon's documentation

```bash
$ babylond testnet \
    --v                     1 \
    --output-dir            <path-to-babylon>/.testnet \
    --starting-ip-address   192.168.10.2 \
    --keyring-backend       test \
    --chain-id              chain-test
$ babylond start --home ./.testnet/node0/babylond
```

3. Launch a vigilante

```bash
$ go run ./cmd/main.go reporter --babylon-key <path-to-babylon>/.testnet/node0/babylond # vigilant reporter
$ go run ./cmd/main.go submitter # vigilant submitter
```

4. Copy the TLS certificate generated and self-signed by btcd to Btcwallet

```bash
$ cp ~/Library/Application\ Support/Btcd/rpc.cert ~/Library/Application\ Support/Btcwallet/rpc.cert # on MacOS
$ cp ~/.btcd/rpc.cert ~/.btcwallet/rpc.cert # on Linux
```

where you might need to create the `Btcwallet` folder manually.

5. Generate a BTC block

```bash
$ btcctl --simnet --wallet --skipverify --rpcuser=user --rpcpass=pass generate 1
```

## Running the vigilante in Docker

One can use the `Dockerfile` to build a Docker image for the vigilant, by using the following CLI:

```bash
$ docker build -t babylonchain/vigilante:latest --build-arg user=<your_Github_username> --build-arg pass=<your_Github_access_token> .
```

where `<your_Github_access_token>` can be generated at [github.com/settings/tokens](https://github.com/settings/tokens).
The Github access token is used for retrieving the `babylonchain/babylon` dependency, which at the moment remains as a private repo.

This `Dockerfile` is also compatible with Docker's [buildx feature](https://docs.docker.com/desktop/multi-arch/) that allows multi-architectural builds. To have a multi-architectural build,

```bash
$ docker buildx create --use
$ docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t babylonchain/vigilante:latest --build-arg user=<your_Github_username> --build-arg pass=<your_Github_access_token> .
```