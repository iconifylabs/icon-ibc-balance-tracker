package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"

	iconclient "github.com/icon-project/goloop/client"
	"github.com/icon-project/goloop/server/jsonrpc"
	v3 "github.com/icon-project/goloop/server/v3"
)

var (
	timeout           = 10 * time.Second
	filePath          = "./wallets.json"
	telegramBotToken  = os.Getenv("TELEGRAM_BOT_TOKEN")
	discordWebhookURL = os.Getenv("DISCORD_WEBHOOK_URL")
)

type NetworkConfig struct {
	Type      string            `json:"type"`
	RPC       string            `json:"rpc"`
	Coin      string            `json:"coin"`
	Name      string            `json:"name"`
	Decimals  int               `json:"decimals"`
	Threshold float64           `json:"threshold"`
	Addresses map[string]string `json:"addresses"`
}

type ChainConfig struct {
	Chains []NetworkConfig `json:"info"`
}

type Balances struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

type CosmosBalance struct {
	Balances []Balances `json:"balances"`
}

type TelegramMessage struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

type DiscordMessage struct {
	Content string `json:"content"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatal(err)
	}

	var chainCfg ChainConfig
	err = json.Unmarshal(content, &chainCfg)
	if err != nil {
		log.Fatal(err)
	}

	for _, networkConfig := range chainCfg.Chains {

		fmt.Printf("Network: %s\n", networkConfig.Name)

		coinName := networkConfig.Coin
		fmt.Printf("%-20s %-22s %-20s\n", "Address", fmt.Sprintf("Balance (%s)", coinName), "Balance")
		fmt.Println(strings.Repeat("-", 64))
		switch networkConfig.Type {
		case "evm":
			client, err := rpc.DialContext(ctx, networkConfig.RPC)
			if err != nil {
				log.Fatal(err)
			}
			defer client.Close()

			for addressName, address := range networkConfig.Addresses {
				balance, err := getETHBalance(client, address)
				if err != nil {
					log.Fatal(err)
				}

				etherBalance := toDecimalUnit(balance, networkConfig.Decimals)
				fmt.Printf("%-20s %-22s %-20s\n", addressName, etherBalance.String(), balance.String())
				if checkBalanceThreshold(balance, networkConfig.Threshold) {
					sendAlert(networkConfig.Name, address, etherBalance.String())
				}
			}

		case "icon":
			client := iconclient.NewClientV3(networkConfig.RPC)
			defer client.Cleanup()

			for addressName, address := range networkConfig.Addresses {
				balance, err := getICXBalance(client, address)
				if err != nil {
					log.Fatal(err)
				}

				icxBalance := toDecimalUnit(balance, networkConfig.Decimals)
				fmt.Printf("%-20s %-22s %-20s\n", addressName, icxBalance.String(), balance.String())
			}

		case "cosmos":
			for addressName, address := range networkConfig.Addresses {
				balance, err := getCosmosBalance(networkConfig.RPC, address, networkConfig.Coin)
				if err != nil {
					log.Fatal(err)
				}

				icxBalance := toDecimalUnit(balance, networkConfig.Decimals)
				fmt.Printf("%-20s %-22s %-20s\n", addressName, icxBalance.String(), balance.String())
				if checkBalanceThreshold(balance, networkConfig.Threshold) {
					sendAlert(networkConfig.Name, address, icxBalance.String())
				}
			}
		}
		fmt.Printf("\n\n")
	}
}

func getCosmosBalance(rpc, address, denom string) (*big.Int, error) {
	apiURL := fmt.Sprintf("%s/cosmos/bank/v1beta1/balances/%s", rpc, address)

	response, err := http.Get(apiURL)
	if err != nil {
		fmt.Println("Error making HTTP request:", err)
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return nil, err
	}

	var cb CosmosBalance
	if err := json.Unmarshal(body, &cb); err != nil {
		fmt.Println("Error unmarshaling:", err)
	}
	for _, c := range cb.Balances {
		if strings.EqualFold(strings.ToUpper(c.Denom), strings.ToUpper(denom)) {
			var bigIntNumber big.Int
			bigIntNumber.SetString(c.Amount, 10)
			return &bigIntNumber, nil
		}
	}
	return big.NewInt(0), nil
}

func getICXBalance(client *iconclient.ClientV3, address string) (*big.Int, error) {
	bal, err := client.GetBalance(&v3.AddressParam{
		Address: jsonrpc.Address(address),
	})
	if err != nil {
		return nil, err
	}
	return bal.BigInt()
}

func getETHBalance(client *rpc.Client, address string) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ethAddress := common.HexToAddress(address)
	var balanceHex string
	err := client.CallContext(ctx, &balanceHex, "eth_getBalance", ethAddress, "latest")
	if err != nil {
		return nil, err
	}

	balance, success := new(big.Int).SetString(strings.TrimPrefix(balanceHex, "0x"), 16)
	if !success {
		return nil, fmt.Errorf("failed to convert balance to big.Int")
	}

	return balance, nil
}

func toDecimalUnit(wei *big.Int, decimals int) *big.Float {
	decimalFactor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)

	ether := new(big.Float).Quo(new(big.Float).SetInt(wei), new(big.Float).SetInt(decimalFactor))
	return ether
}

// check if balance is below threshold
func checkBalanceThreshold(balance *big.Int, threshold float64) bool {
	balanceFloat := toDecimalUnit(balance, 18)
	return balanceFloat.Cmp(big.NewFloat(threshold)) == -1
}

// send alert if balance is below threshold
func sendAlert(network, address, balance string) {
	message := fmt.Sprintf("Low balance alert for %s address %s. Current balance is %s", network, address, balance)
	sendTelegramAlert(message)
	sendDiscordAlert(message)
}

func sendTelegramAlert(message string) error {
	msg := TelegramMessage{
		Text: message,
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = http.Post("https://api.telegram.org/bot"+telegramBotToken+"/sendMessage", "application/json", bytes.NewBuffer(jsonMsg))
	return err
}

func sendDiscordAlert(message string) error {
	msg := DiscordMessage{
		Content: message,
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	resp, err := http.Post(discordWebhookURL, "application/json", bytes.NewBuffer(jsonMsg))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
