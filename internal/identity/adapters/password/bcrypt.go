package password

import "golang.org/x/crypto/bcrypt"

type Bcrypt struct{ Cost int }

func (hasher Bcrypt) Hash(value string) (string, error) {
	cost := hasher.Cost
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(value), cost)
	return string(hashed), err
}

func (Bcrypt) Compare(hashed, value string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(value))
}
