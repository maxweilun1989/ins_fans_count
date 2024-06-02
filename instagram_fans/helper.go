package instagram_fans

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseFollowerCount(s string) (int, error) {
	// 去掉字符串中的空格
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")

	// 检查字符串的最后一个字符以确定单位
	length := len(s)
	if length == 0 {
		return 0, fmt.Errorf("invalid format")
	}

	unit := s[length-1]
	numberStr := s[:length-1]
	var multiplier int

	switch unit {
	case 'K', 'k':
		multiplier = 1000
	case 'M', 'm':
		multiplier = 1000000
	case 'B', 'b':
		multiplier = 1000000000
	default:
		// 如果没有单位，尝试直接转换为整数
		numberStr = s
		multiplier = 1
	}

	// 将数字部分转换为浮点数
	number, err := strconv.ParseFloat(numberStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number format: %v", err)
	}

	// 计算实际的粉丝数
	followers := int(number * float64(multiplier))
	return followers, nil
}
