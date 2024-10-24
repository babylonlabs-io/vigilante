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
