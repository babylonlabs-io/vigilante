// SPDX-License-Identifier: MIT
pragma solidity >=0.8.0;

error WrongHeaderLength();
error NoBlocksSubmitted();
error BadParent();
error NoParent();
error HashAboveTarget();
error DifficultyRetargetLT25(); // <25% difficulty retarget
error WrongDifficultyBits();
error OldDifficultyPeriod();
error InsufficientTotalDifficulty();
error InsufficientChainLength();
error TooDeepReorg();

/**
 * @notice Tracks Bitcoin. Provides block hashes.
 */
interface IBtcPrism {
    /**
     * @notice Returns the Bitcoin block hash at a specific height.
     */
    function getBlockHash(uint256 number) external view returns (bytes32);

    /**
     * @notice Returns the height of the latest block (tip of the chain).
     */
    function getLatestBlockHeight() external view returns (uint256);

    /**
     * @notice Returns the timestamp of the lastest block, as Unix seconds.
     */
    function getLatestBlockTime() external view returns (uint256);

    /**
     * @notice Reverse lookup: returns the block height for a given block hash
     * @param blockHash The block hash to look up
     * @return blockNumber The height of the block, or 0 if not found in recent history
     * @dev Only searches within the last MAX_ALLOWED_REORG blocks
     */
    function getBlockNumber(bytes32 blockHash) external view returns (uint256);

    /**
     * @notice Submits a new Bitcoin chain segment (80-byte headers) s
     */
    function submit(uint256 blockHeight, bytes calldata blockHeaders) external;
}
