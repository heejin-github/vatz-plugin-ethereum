package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "math/big"
    "github.com/rs/zerolog/log"
    "github.com/go-resty/resty/v2"
    pluginpb "github.com/dsrvlabs/vatz-proto/plugin/v1"
    "github.com/dsrvlabs/vatz/sdk"
    "golang.org/x/net/context"
    "google.golang.org/protobuf/types/known/structpb"
)

const (
    defaultAddr          = "127.0.0.1"
    defaultPort          = 10001
    pluginName           = "vatz-plugin-ethereum-block-height"
    defaultCriticalCount = 3
)

var (
    addr          string
    port          int
    prevHeight    int64
    warningCount  int
    criticalCount int
)

func init() {
    flag.StringVar(&addr, "addr", defaultAddr, "Listening address")
    flag.IntVar(&port, "port", defaultPort, "Listening port")
    flag.IntVar(&criticalCount, "critical", defaultCriticalCount, "Block height stuck count to raise critical level of alert")
    flag.Parse()
}

func main() {
    p := sdk.NewPlugin(pluginName)
    p.Register(pluginFeature)

    ctx := context.Background()
    if err := p.Start(ctx, addr, port); err != nil {
        log.Info().Str("module", "plugin").Msg("exit")
    }
}

type BlockHeightResponse struct {
    JsonRPC string `json:"jsonrpc"`
    Result  string `json:"result"`
    ID      int    `json:"id"`
}

func pluginFeature(info, option map[string]*structpb.Value) (sdk.CallResponse, error) {
    ret := sdk.CallResponse{
        FuncName: info["execute_method"].GetStringValue(),
        Message:  "Unable to fetch block height",
        Severity: pluginpb.SEVERITY_UNKNOWN,
        State:    pluginpb.STATE_NONE,
    }

    client := resty.New()
    url := "http://localhost:8545"

    body := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      1,
        "method":  "eth_blockNumber",
        "params":  []string{},
    }

    resp, err := client.R().
        SetHeader("Content-Type", "application/json").
        SetBody(body).
        Post(url)

    if err != nil {
        log.Error().Str("module", "plugin").Msgf("failed to get response: %v", err)
        ret.Message = fmt.Sprintf("Failed to get response: %v", err)
        ret.Severity = pluginpb.SEVERITY_CRITICAL
        ret.State = pluginpb.STATE_FAILURE
        return ret, err
    }

    var blockHeightResp BlockHeightResponse
    err = json.Unmarshal(resp.Body(), &blockHeightResp)
    if err != nil {
        log.Error().Str("module", "plugin").Msgf("failed to parse response: %v", err)
        ret.Message = fmt.Sprintf("Failed to parse response: %v", err)
        ret.Severity = pluginpb.SEVERITY_CRITICAL
        ret.State = pluginpb.STATE_FAILURE
        return ret, err
    }

    // Convert hex string to int64
    latestHeight, err := hexToInt64(blockHeightResp.Result)
    if err != nil {
        log.Error().Str("module", "plugin").Msgf("failed to convert hex to int64: %v", err)
        ret.Message = fmt.Sprintf("Failed to convert hex to int64: %v", err)
        ret.Severity = pluginpb.SEVERITY_CRITICAL
        ret.State = pluginpb.STATE_FAILURE
        return ret, err
    }

    log.Info().Str("module", "plugin").Msgf("Previous block height: %d, Latest block height: %d", prevHeight, latestHeight)

    if latestHeight > prevHeight {
        ret.Message = fmt.Sprintf("Block height increasing. Current height: %d", latestHeight)
        ret.Severity = pluginpb.SEVERITY_INFO
        warningCount = 0
    } else {
        warningCount++
        if warningCount > criticalCount {
            ret.Message = fmt.Sprintf("Block height stuck more than %d times. Current height: %d", criticalCount, latestHeight)
            ret.Severity = pluginpb.SEVERITY_CRITICAL
        } else {
            ret.Message = fmt.Sprintf("Block height stuck %d times. Current height: %d", warningCount, latestHeight)
            ret.Severity = pluginpb.SEVERITY_WARNING
        }
    }

    ret.State = pluginpb.STATE_SUCCESS
    log.Debug().Str("module", "plugin").Msg(ret.Message)

    prevHeight = latestHeight
    return ret, nil
}

func hexToInt64(hexStr string) (int64, error) {
    // Remove "0x" prefix if present
    if len(hexStr) >= 2 && hexStr[:2] == "0x" {
        hexStr = hexStr[2:]
    }
    
    // Parse hex string to big.Int
    n := new(big.Int)
    n, ok := n.SetString(hexStr, 16)
    if !ok {
        return 0, fmt.Errorf("failed to parse hex string: %s", hexStr)
    }
    
    // Convert big.Int to int64
    if !n.IsInt64() {
        return 0, fmt.Errorf("hex value too large for int64: %s", hexStr)
    }
    
    return n.Int64(), nil
}
