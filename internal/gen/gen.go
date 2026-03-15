package gen

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	genCmd := &cobra.Command{
		Use:   "gen",
		Short: "Generate UUIDs, passwords, and random strings",
		Long: `Generate random values for common developer needs.

  babi gen uuid                      # random UUID v4
  babi gen uuid -n 5                 # generate 5 UUIDs
  babi gen pass                      # 20-char password
  babi gen pass -l 32 -s             # 32-char with symbols
  babi gen str                       # 16-char alphanumeric string
  babi gen str -l 32 -c hex          # 32-char hex string
  babi gen str -l 16 -c alpha        # 16-char alphabetic`,
	}

	var genCount int

	uuidCmd := &cobra.Command{
		Use: "uuid", Short: "Generate UUID v4",
		RunE: func(cmd *cobra.Command, args []string) error {
			n := genCount
			if n <= 0 {
				n = 1
			}
			for i := 0; i < n; i++ {
				u, err := newUUID()
				if err != nil {
					return err
				}
				fmt.Println(u)
			}
			return nil
		},
	}
	uuidCmd.Flags().IntVarP(&genCount, "count", "n", 1, "number of values to generate")

	var genPassLen int
	var genPassSymbols bool
	var passCount int
	passCmd := &cobra.Command{
		Use: "pass", Short: "Generate a random password",
		RunE: func(cmd *cobra.Command, args []string) error {
			n := passCount
			if n <= 0 {
				n = 1
			}
			l := genPassLen
			if l <= 0 {
				l = 20
			}
			charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
			if genPassSymbols {
				charset += "!@#$%^&*-_=+?"
			}
			for i := 0; i < n; i++ {
				s, err := randString(charset, l)
				if err != nil {
					return err
				}
				fmt.Println(s)
			}
			return nil
		},
	}
	passCmd.Flags().IntVarP(&genPassLen, "length", "l", 20, "password length")
	passCmd.Flags().BoolVarP(&genPassSymbols, "symbols", "s", false, "include symbols (!@#$%^&*-_=+?)")
	passCmd.Flags().IntVarP(&passCount, "count", "n", 1, "number of passwords to generate")

	var genStrLen int
	var genStrCharset string
	var strCount int
	strCmd := &cobra.Command{
		Use: "str", Short: "Generate a random string",
		RunE: func(cmd *cobra.Command, args []string) error {
			n := strCount
			if n <= 0 {
				n = 1
			}
			l := genStrLen
			if l <= 0 {
				l = 16
			}
			var charset string
			switch strings.ToLower(genStrCharset) {
			case "hex":
				charset = "0123456789abcdef"
			case "alpha":
				charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
			case "num", "numeric":
				charset = "0123456789"
			default:
				charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
			}
			for i := 0; i < n; i++ {
				s, err := randString(charset, l)
				if err != nil {
					return err
				}
				fmt.Println(s)
			}
			return nil
		},
	}
	strCmd.Flags().IntVarP(&genStrLen, "length", "l", 16, "string length")
	strCmd.Flags().StringVarP(&genStrCharset, "charset", "c", "alphanum", "charset: alphanum, alpha, hex, num")
	strCmd.Flags().IntVarP(&strCount, "count", "n", 1, "number of strings to generate")

	genCmd.AddCommand(uuidCmd, passCmd, strCmd)
	return genCmd
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func randString(charset string, length int) (string, error) {
	max := big.NewInt(int64(len(charset)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = charset[n.Int64()]
	}
	return string(out), nil
}
