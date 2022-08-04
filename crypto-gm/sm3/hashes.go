package sm3

import (
	"hash"
)

func NewKeccak256() hash.Hash { return New() }

// Sum256 returns the SM3 digest of the data.
func Sum256(data []byte) []byte {
	hash := Sm3Sum(data)
	res := make([]byte, len(hash))
	copy(res[:], hash)
	return res
}

// Sum calculate data into hash
func Sum(hash, data []byte) {
	tmp := Sm3Sum(data)
	copy(hash, tmp)
}
