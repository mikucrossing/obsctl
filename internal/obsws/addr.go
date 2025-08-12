package obsws

import "strings"

// NormalizeObsAddr は goobs.New に渡すために、ユーザーが ws:// や wss:// を付けてしまった場合に除去します。
func NormalizeObsAddr(a string) string {
    a = strings.TrimSpace(a)
    if strings.HasPrefix(a, "ws://") {
        return strings.TrimPrefix(a, "ws://")
    }
    if strings.HasPrefix(a, "wss://") {
        return strings.TrimPrefix(a, "wss://")
    }
    return a
}

