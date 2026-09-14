package rankings

// Fixed aggregate-only SQL capabilities own cross-domain source reads. The
// runtime never needs raw Poker, hidden-card or cross-user History grants.
const gameAggregateSQL = `SELECT rankings.build_game_entries($1::uuid)`
const rpAggregateSQL = `SELECT rankings.build_rp_entries($1::uuid,$2::uuid)`