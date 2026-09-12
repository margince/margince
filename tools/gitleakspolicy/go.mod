// The secret-scan policy decoder, and the one reason it is its own module
// rather than a package under backend/tools: backend/tools is a member of the
// repo's Go workspace, and a dependency added there lands in go.work.sum, whose
// materialized copy the composition generator hashes for reproducibility. A
// TOML decoder has nothing to do with the composed build, and making the
// composition's fingerprint depend on it would be the tail wagging the dog.
//
// Nothing here ships in a binary a customer runs. It is built by
// scripts/test-secret-scan.sh, which proves the credential gate's allowlists
// are narrow.
module github.com/margince/margince/tools/gitleakspolicy

go 1.26.6

require github.com/pelletier/go-toml/v2 v2.3.1
