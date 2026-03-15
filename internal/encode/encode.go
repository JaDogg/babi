package encode

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	encodeCmd := &cobra.Command{
		Use:   "encode",
		Short: "Encode and decode data (base64, hex, URL)",
		Long: `Encode or decode data. Reads from arguments or stdin.

  babi encode b64   "hello"           # base64 encode
  babi encode b64d  "aGVsbG8="        # base64 decode
  babi encode hex   "hello"           # hex encode
  babi encode hexd  "68656c6c6f"      # hex decode
  babi encode url   "hello world"     # URL encode
  babi encode urld  "hello%20world"   # URL decode
  echo "hello" | babi encode b64      # read from stdin`,
	}

	encodeCmd.AddCommand(
		&cobra.Command{
			Use: "b64 [data]", Short: "Encode to base64",
			Args: cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				data, err := readInput(args)
				if err != nil {
					return err
				}
				fmt.Println(base64.StdEncoding.EncodeToString(data))
				return nil
			},
		},
		&cobra.Command{
			Use: "b64d [data]", Short: "Decode from base64",
			Args: cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				data, err := readInput(args)
				if err != nil {
					return err
				}
				out, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
				if err != nil {
					out, err = base64.URLEncoding.DecodeString(strings.TrimSpace(string(data)))
				}
				if err != nil {
					return fmt.Errorf("invalid base64: %w", err)
				}
				fmt.Print(string(out))
				return nil
			},
		},
		&cobra.Command{
			Use: "hex [data]", Short: "Encode to hex",
			Args: cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				data, err := readInput(args)
				if err != nil {
					return err
				}
				fmt.Println(hex.EncodeToString(data))
				return nil
			},
		},
		&cobra.Command{
			Use: "hexd [data]", Short: "Decode from hex",
			Args: cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				data, err := readInput(args)
				if err != nil {
					return err
				}
				out, err := hex.DecodeString(strings.TrimSpace(string(data)))
				if err != nil {
					return fmt.Errorf("invalid hex: %w", err)
				}
				fmt.Print(string(out))
				return nil
			},
		},
		&cobra.Command{
			Use: "url [data]", Short: "URL-encode a string",
			Args: cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				data, err := readInput(args)
				if err != nil {
					return err
				}
				fmt.Println(url.QueryEscape(string(data)))
				return nil
			},
		},
		&cobra.Command{
			Use: "urld [data]", Short: "URL-decode a string",
			Args: cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				data, err := readInput(args)
				if err != nil {
					return err
				}
				out, err := url.QueryUnescape(strings.TrimSpace(string(data)))
				if err != nil {
					return fmt.Errorf("invalid URL encoding: %w", err)
				}
				fmt.Println(out)
				return nil
			},
		},
	)
	return encodeCmd
}

func readInput(args []string) ([]byte, error) {
	if len(args) > 0 {
		return []byte(strings.Join(args, " ")), nil
	}
	return io.ReadAll(os.Stdin)
}
