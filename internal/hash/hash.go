package hash

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	stdhash "hash"
	"os"
	"strings"

	cc "github.com/jadogg/babi/internal/clicolor"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	var hashAlgo string
	var hashString string

	cmd := &cobra.Command{
		Use:   "hash [file...]",
		Short: "Hash files or strings",
		Long: `Hash files or strings using common algorithms.
Reads from stdin if no file is given.

  babi hash file.zip                    # sha256 (default)
  babi hash -a md5 file.zip             # md5
  babi hash -a sha1 -s "hello world"    # sha1 of a string
  babi hash *.go                        # hash multiple files`,
		RunE: func(cmd *cobra.Command, args []string) error {
			h, name, err := newHasher(hashAlgo)
			if err != nil {
				return err
			}
			if hashString != "" {
				h.Write([]byte(hashString))
				fmt.Printf("%s  %s (%s)\n", hex.EncodeToString(h.Sum(nil)), cc.Dim(`"`+hashString+`"`), name)
				return nil
			}
			if len(args) == 0 {
				if _, err := io.Copy(h, os.Stdin); err != nil {
					return err
				}
				fmt.Printf("%s  -\n", hex.EncodeToString(h.Sum(nil)))
				return nil
			}
			for _, path := range args {
				hh, _, _ := newHasher(hashAlgo)
				f, err := os.Open(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s: %v\n", cc.BoldRed("error"), err)
					continue
				}
				io.Copy(hh, f)
				f.Close()
				fmt.Printf("%s  %s\n", hex.EncodeToString(hh.Sum(nil)), path)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&hashAlgo, "algo", "a", "sha256", "algorithm: md5, sha1, sha224, sha256, sha384, sha512")
	cmd.Flags().StringVarP(&hashString, "string", "s", "", "hash a string instead of a file")
	return cmd
}

func newHasher(algo string) (stdhash.Hash, string, error) {
	switch strings.ToLower(algo) {
	case "", "sha256":
		return sha256.New(), "sha256", nil
	case "sha512":
		return sha512.New(), "sha512", nil
	case "sha1":
		return sha1.New(), "sha1", nil
	case "sha224":
		return sha256.New224(), "sha224", nil
	case "sha384":
		return sha512.New384(), "sha384", nil
	case "md5":
		return md5.New(), "md5", nil
	}
	return nil, "", fmt.Errorf("unknown algorithm %q — use: md5, sha1, sha224, sha256, sha384, sha512", algo)
}
