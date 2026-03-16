// Package ean provides EAN-13 barcode generation and validation.
package ean

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// GenerateEAN13 generates a random, algorithmically valid EAN-13 barcode.
// Uses crypto/rand for randomness. The check digit is calculated per ISO/IEC 15420.
func GenerateEAN13() (string, error) {
	digits := make([]byte, 12)
	for i := range digits {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("generating random digit: %w", err)
		}
		digits[i] = byte(n.Int64())
	}

	check := checkDigit(digits)

	result := make([]byte, 13)
	for i, d := range digits {
		result[i] = '0' + d
	}
	result[12] = '0' + check
	return string(result), nil
}

// ValidateEAN13 checks that a 13-digit string has a valid EAN-13 check digit.
func ValidateEAN13(code string) bool {
	if len(code) != 13 {
		return false
	}
	digits := make([]byte, 12)
	for i := 0; i < 12; i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
		digits[i] = code[i] - '0'
	}
	if code[12] < '0' || code[12] > '9' {
		return false
	}
	expected := checkDigit(digits)
	return code[12]-'0' == expected
}

// checkDigit calculates the EAN-13 check digit from the first 12 digits.
// Uses alternating weights of 1 and 3.
func checkDigit(digits []byte) byte {
	sum := 0
	for i, d := range digits {
		if i%2 == 0 {
			sum += int(d)
		} else {
			sum += int(d) * 3
		}
	}
	return byte((10 - (sum % 10)) % 10)
}
