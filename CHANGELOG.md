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

## v0.19.9

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
