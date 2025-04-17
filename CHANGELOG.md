<!--
Guiding Principles:

Changelogs are for humans, not machines.
There should be an entry for every single version.
The same types of changes should be grouped.
Versions and sections should be linkable.
The latest version comes first.
The release date of each version is displayed.
Mention whether you follow Semantic Versioning.

Usage:

Change log entries are to be added to the Unreleased section under the
appropriate stanza (see below). Each entry should have following format:

* [#PullRequestNumber](PullRequestLink) message

Types of changes (Stanzas):

"Features" for new features.
"Improvements" for changes in existing functionality.
"Deprecated" for soon-to-be removed features.
"Bug Fixes" for any bug fixes.
"Client Breaking" for breaking CLI commands and REST routes used by end-users.
"API Breaking" for breaking exported APIs used by developers building on SDK.
"State Machine Breaking" for any changes that result in a different AppState
given same genesisState and txList.
Ref: https://keepachangelog.com/en/1.0.0/
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)

## Unreleased

### Improvements

* [#331](https://github.com/babylonlabs-io/vigilante/pull/331) feat: adds send raw tx with burn amount

## v0.23.3

### Bug Fixes
* [#315](https://github.com/babylonlabs-io/vigilante/pull/315) fix: default config saving in `dump-cfg` command

### Improvements

* [#303](https://github.com/babylonlabs-io/vigilante/pull/303) chore: sew reduce logs
* [#308](https://github.com/babylonlabs-io/vigilante/pull/308) chore: reporter handle duplicate submissions
* [#318](https://github.com/babylonlabs-io/vigilante/pull/318) fix: start metrics server before blocking reporter execution
* [#323](https://github.com/babylonlabs-io/vigilante/pull/323) feat: improve reporter bootstrapping performance and reliability in checkpoint submission

## v0.23.2

### Improvements

* [#304](https://github.com/babylonlabs-io/vigilante/pull/304) chore: bump bbn v1.0.0

## v0.23.1

### Improvements

* [#294](https://github.com/babylonlabs-io/vigilante/pull/294) chore: version cmd
* [#295](https://github.com/babylonlabs-io/vigilante/pull/295) chore: remove grpc server
* [#258](https://github.com/babylonlabs-io/vigilante/pull/258) fix: Reject non-negative value and Zero for time Interval used by time.ticker
* [#296](https://github.com/babylonlabs-io/vigilante/pull/296) chore: tm retry container start
* [#297](https://github.com/babylonlabs-io/vigilante/pull/297) chore: unbonding watcher block metrics
* [#298](https://github.com/babylonlabs-io/vigilante/pull/298) chore: log binary version

## v0.23.0

### Improvements

* [#278](https://github.com/babylonlabs-io/vigilante/pull/278) chore: metrics for censorship detection
* [#271](https://github.com/babylonlabs-io/vigilante/pull/271) chore: sync the sample config
* [#289](https://github.com/babylonlabs-io/vigilante/pull/289) chore: additional check for getFundingtx
* [#290](https://github.com/babylonlabs-io/vigilante/pull/290) chore: dump config command
* [#291](https://github.com/babylonlabs-io/vigilante/pull/291) chore: bump bbn to rc.9

## v0.22.1

### Bug Fixes

* [#271](https://github.com/babylonlabs-io/vigilante/pull/271) fix: support old and new genesis formats

## v0.22.0

### Improvements

* [#255](https://github.com/babylonlabs-io/vigilante/pull/255) chore: disable tls in config
* [#257](https://github.com/babylonlabs-io/vigilante/pull/257) chore: change default max
* [#260](https://github.com/babylonlabs-io/vigilante/pull/260) chore: add fetch-evidence-interval to sample config
* [#262](https://github.com/babylonlabs-io/vigilante/pull/262) chore: submitter e2e for tx values bellow dust
* [#263](https://github.com/babylonlabs-io/vigilante/pull/260) chore: Update zmq endpoints in sample config
* [#265](https://github.com/babylonlabs-io/vigilante/pull/265) chore: unit tests for relayer
* [#266](https://github.com/babylonlabs-io/vigilante/pull/266) chore: bump bbn to rc8

### Bug Fixes

* [#250](https://github.com/babylonlabs-io/vigilante/pull/250) fix: handle rpc errors in maybeResendFromStore
* [#252](https://github.com/babylonlabs-io/vigilante/pull/252) fix: rbf compliant
* [#257](https://github.com/babylonlabs-io/vigilante/pull/257) fix: rbf fee calculation
* [#241](https://github.com/babylonlabs-io/vigilante/pull/241) chore: limit response read
* [#256](https://github.com/babylonlabs-io/vigilante/pull/256) fix: genesis parsing
* [#236](https://github.com/babylonlabs-io/vigilante/pull/236) fix: handle has inclusion proof err
* [#261](https://github.com/babylonlabs-io/vigilante/pull/261) fix: tweak fund tx

## v0.21.0

### Improvements

* [#249](https://github.com/babylonlabs-io/vigilante/pull/249) chore: adapt unbonding to new babylon version

## v0.20.0

### Improvements

* [#211](https://github.com/babylonlabs-io/vigilante/pull/211) feat: adds indexer to btc staking tracker
* [#229](https://github.com/babylonlabs-io/vigilante/pull/229) chore: bigger batch size for fetching delegations
* [#233](https://github.com/babylonlabs-io/vigilante/pull/233) chore: cleanup ckpt cache
* [#235](https://github.com/babylonlabs-io/vigilante/pull/233) chore: use file based lock to allocate ports
* [#232](https://github.com/babylonlabs-io/vigilante/pull/232) chore: comply to rbf policy
* [#224](https://github.com/babylonlabs-io/vigilante/pull/224) chore: poll for evidence

### Bug Fixes

* [#209](https://github.com/babylonlabs-io/vigilante/pull/209) fix: wait until slashing tx k-deep
* [#223](https://github.com/babylonlabs-io/vigilante/pull/223) fix: consider minimal fee for bump
* [#226](https://github.com/babylonlabs-io/vigilante/pull/226) fix: reselect inputs after adding manual output
* [#229](https://github.com/babylonlabs-io/vigilante/pull/229) chore: lax unecessary btc tx checks
* [#237](https://github.com/babylonlabs-io/vigilante/pull/237) fix: send on closed chan

## v0.19.9

### Improvements

* [#211](https://github.com/babylonlabs-io/vigilante/pull/211) feat: adds indexer to btc staking tracker

### Bug Fixes

* [#202](https://github.com/babylonlabs-io/vigilante/pull/202) fix: bump down btcd

## v0.19.8

### Improvements

* [#194](https://github.com/babylonlabs-io/vigilante/pull/194) fix: reduce locks
* [#195](https://github.com/babylonlabs-io/vigilante/pull/195) chore: bump bbn to rc4
* [#196](https://github.com/babylonlabs-io/vigilante/pull/196) fix: reporter ensure bootstrap happens on error

## v0.19.7

### Improvements

* [#190](https://github.com/babylonlabs-io/vigilante/pull/190) chore: additional metric for tracker, less logs

## v0.19.6

### Bug Fixes

* [#184](https://github.com/babylonlabs-io/vigilante/pull/184) fix: removing from metrics tracker

## v0.19.5

### Improvements

* [#178](https://github.com/babylonlabs-io/vigilante/pull/178) chore: better bucket range
* [#179](https://github.com/babylonlabs-io/vigilante/pull/179) chore: metrics for bs tracker

## v0.19.4

### Bug Fixes

* [#171](https://github.com/babylonlabs-io/vigilante/pull/171) fix: delegation iter

## v0.19.3

### Bug Fixes

* [#166](https://github.com/babylonlabs-io/vigilante/pull/166) fix: refactor checkpoint tx submission,
fix change addr creation

## v0.19.2

### Bug Fixes

* [#160](https://github.com/babylonlabs-io/vigilante/pull/160) fix: resubmit interval

## v0.19.1

### Bug Fixes

* [#154](https://github.com/babylonlabs-io/vigilante/pull/154) fix: panic in maybeResendSecondTxOfCheckpointToBTC

### Improvements

* [#155](https://github.com/babylonlabs-io/vigilante/pull/155) chore: increase retry attempts for header reporter

## v0.19.0

### Bug Fixes

* [#138](https://github.com/babylonlabs-io/vigilante/pull/138) fix: panic in SendCheckpointToBTC

### Improvements

* [#139](https://github.com/babylonlabs-io/vigilante/pull/139) add opcc slashing event
* [#136](https://github.com/babylonlabs-io/vigilante/pull/136) rate limit activations
* [#141](https://github.com/babylonlabs-io/vigilante/pull/141) decrement tracked delegations in atomic slasher
* [#143](https://github.com/babylonlabs-io/vigilante/pull/143) adds nlreturn linter rule
* [#145](https://github.com/babylonlabs-io/vigilante/pull/145) fix: tracked delegation mutex
* [#147](https://github.com/babylonlabs-io/vigilante/pull/147) babylon to v1.0.0-rc.1

## v0.18.0

### Improvements

* [#132](https://github.com/babylonlabs-io/vigilante/pull/132) bump bbn v0.18.0

## v0.17.3

### Improvements

* [#127](https://github.com/babylonlabs-io/vigilante/pull/127) fix long lock time

## v0.17.2

### Improvements

* [#123](https://github.com/babylonlabs-io/vigilante/pull/123) more metrics for bstracker

## v0.17.1

### Improvements

* [#113](https://github.com/babylonlabs-io/vigilante/pull/113) goreleaser wasm version
* [#115](https://github.com/babylonlabs-io/vigilante/pull/115) updates bbn to v0.17.1


## v0.17.0

### Improvements

* [#100](https://github.com/babylonlabs-io/vigilante/pull/100) bump docker workflow to 0.10.2,
fix some dockerfile issue
* [#111](https://github.com/babylonlabs-io/vigilante/pull/111) updates bbn to v0.17.0

## v0.16.1

### Improvements

* [#105](https://github.com/babylonlabs-io/vigilante/pull/105) Measure latency
* [#106](https://github.com/babylonlabs-io/vigilante/pull/106) Wait for stacking tx to be k-deep


## v0.16.0

* [#94](https://github.com/babylonlabs-io/vigilante/pull/94) adds gosec and fixes gosec issues
* [#96](https://github.com/babylonlabs-io/vigilante/pull/96) fixes potential stale data read
* [#98](https://github.com/babylonlabs-io/vigilante/pull/98) fixes golangci configuration
* [#102](https://github.com/babylonlabs-io/vigilante/pull/102) babylon v0.16.0 upgrade

## v0.15.0

* [#90](https://github.com/babylonlabs-io/vigilante/pull/90) upgrade babylon to v0.15.0

## v0.14.0

### Bug Fixes

* [#84](https://github.com/babylonlabs-io/vigilante/pull/84) fix spawning more go routines than needed when activating 
delegations, add more logging

### Improvements
* [#87](https://github.com/babylonlabs-io/vigilante/pull/87) adr 029 for generalized unbonding

## v0.13.0

### Improvements

* [#80](https://github.com/babylonlabs-io/vigilante/pull/80) bump babylon to use
uint32 in BTC block heights and BLS valset response
* [#79](https://github.com/babylonlabs-io/vigilante/pull/79) handle no change output when building tx
* [#77](https://github.com/babylonlabs-io/vigilante/pull/77) add arm64 static build
* [#76](https://github.com/babylonlabs-io/vigilante/pull/76) add goreleaser
  setup and move out changelog reminder
