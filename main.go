package main

import (
	"github.com/withmandala/go-log"
	"math/rand"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Version of Program
const Version = "1.0"

var sessdata string
var csrf string

var logger = log.New(os.Stderr)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// 15~30 秒随机等待, 避免固定节奏被风控识别
func randomDelay() time.Duration {
	return time.Duration(15+rng.Intn(16)) * time.Second
}

func main() {
	logger.Info("Version	", Version, "	Running...")
	split()
	// 凭证获取优先级: 命令行参数 > 本地 cookies.txt > 扫码登录
	if len(os.Args) >= 3 {
		csrf = os.Args[1]
		sessdata = os.Args[2]
		logger.Info("使用命令行参数提供的凭证")
	} else {
		if c, s, err := loadCookies(); err == nil {
			csrf, sessdata = c, s
			logger.Info("已从 ", cookieFile, " 载入凭证")
		} else {
			c, s, err := qrLogin()
			if err != nil {
				logger.Fatal("扫码登录失败: ", err.Error())
			}
			csrf, sessdata = c, s
			saveCookies(csrf, sessdata)
			logger.Info("登录成功, 凭证已保存到 ", cookieFile, ", 下次启动免扫码")
		}
	}
	split()
	fetchBuvid()
	juryInfo, err := getJuryInfo()
	if err != nil {
		logger.Fatal("failed to fetch jury info:", err.Error())
	}
	logger.Info("Account Info")
	split()
	logger.Info("Account:	", juryInfo.Uname)
	logger.Info("CaseTotal:	", juryInfo.CaseTotal)
	logger.Info("Status:	", juryInfo.Status)
	logger.Info("TermEnd:	", time.Unix(int64(juryInfo.TermEnd), 0).Format("2006-01-02 15:04:05"))
	split()
	if juryInfo.Status != 1 {
		logger.Fatal("you are not in jury.")
	}
	// 连续因意见不足而跳过的案件数, 用于深夜无活时的退避
	skipCount := 0
	// 跟投所需的最少意见数; 原版为 10, 凌晨新案意见稀少根本凑不齐, 降为 8
	const minVotes = 8
	for {
		logger.Info("fetching next case...")
		split()
		caseID, err := getNext()
		if err != nil {
			logger.Fatal("failed to fetch next case:", err.Error())
		}
		caseInfo, err := getCaseInfo(caseID)
		if err != nil {
			logger.Fatal("failed to fetch case info:", err.Error())
		}
		voteItem := buildVoteItem(caseInfo)

		logger.Info("CaseID:	", caseID)
		logger.Info("CaseType:	", caseInfo.CaseType)
		split()
		logger.Info("fetching opinions...")
		split()

		opinion, opErr := getFullOpinion(caseID)

		retryTimes := 0
		for opErr != nil || len(opinion.List) < minVotes {
			if retryTimes >= 5 {
				skipCount++
				logger.Infof("Retried 5 times! fetch another case... (%d cases skipped in a row)", skipCount)
				if skipCount >= 10 {
					logger.Info("Too many cases lack opinions (probably night time). Sleeping 5 minutes...")
					time.Sleep(5 * time.Minute)
					skipCount = 0
				}
				caseID, err = getNext()
				if err != nil {
					logger.Fatal("failed to fetch next case:", err.Error())
				}
				// 换案后必须同步刷新 caseInfo 和 voteItem, 否则会用旧案件的投票选项
				caseInfo, err = getCaseInfo(caseID)
				if err != nil {
					logger.Fatal("failed to fetch case info:", err.Error())
				}
				voteItem = buildVoteItem(caseInfo)
				split()
				logger.Info("CaseID:	", caseID)
				logger.Info("CaseType:	", caseInfo.CaseType)
				split()
				opinion, opErr = getFullOpinion(caseID)
				retryTimes = 0
			} else {
				if opErr != nil {
					logger.Warn("failed to fetch opinions: ", opErr.Error())
				} else {
					logger.Infof("Votes amount less than %d (current: %d), retry fetching later...", minVotes, len(opinion.List))
				}
				time.Sleep(time.Duration(8+rng.Intn(8)) * time.Second)
				retryTimes++
				opinion, opErr = getFullOpinion(caseID)
			}
		}
		voteMap := map[int]int{}
		for _, s := range opinion.List {
			voteMap[s.Vote] = voteMap[s.Vote] + 1
		}
		maxKey := 0
		maxValue := 0
		for k, v := range voteMap {
			if v > maxValue {
				maxKey = k
				maxValue = v
			}
		}
		// 多数派不足 60% 时意见分歧大, 真人会犹豫, 跳过该案
		if maxValue*100 < len(opinion.List)*60 {
			skipCount++
			logger.Infof("意见分歧大 (多数派 %d/%d), 跳过该案... (%d cases skipped in a row)", maxValue, len(opinion.List), skipCount)
			if skipCount >= 10 {
				logger.Info("Too many cases skipped in a row. Sleeping 5 minutes...")
				time.Sleep(5 * time.Minute)
				skipCount = 0
			}
			split()
			continue
		}
		// 20% 概率随机复制一条同立场非空意见作为自己的投票观点
		content := ""
		if rng.Intn(100) < 20 {
			var candidates []int
			for i, s := range opinion.List {
				if s.Vote == maxKey && s.Content != "" {
					candidates = append(candidates, i)
				}
			}
			if len(candidates) > 0 {
				picked := opinion.List[candidates[rng.Intn(len(candidates))]]
				content = picked.Content
				logger.Info("携带投票观点 (复制自意见 opid=", picked.Opid, ")")
			}
		}
		logger.Info("VotedKey:		", maxKey)
		logger.Info("VotedTimes:		", maxValue)
		logger.Info("TotalVotedTimes:	", opinion.Total)
		logger.Info("VoteText:		", voteItem[maxKey])
		split()
		// 按被举报内容长度模拟阅读时间(约每秒 8 字), 夹在 2~15 秒
		readSec := utf8.RuneCountInString(caseInfo.CaseInfo.Comment.Content) / 8
		if readSec < 2 {
			readSec = 2
		}
		if readSec > 15 {
			readSec = 15
		}
		logger.Infof("reading case for %ds...", readSec)
		time.Sleep(time.Duration(readSec) * time.Second)
		logger.Info("claiming case...")
		_, err = postVote(caseID, 0, "")
		if err != nil {
			logger.Fatal("failed to claim case:", err.Error())
		}
		// 投票冷却取官方 vote_cd 与随机值中的较大者
		d := randomDelay()
		if cd := time.Duration(caseInfo.VoteCd) * time.Second; cd > d {
			d = cd
		}
		logger.Infof("claim succeeded. sleeping for %v...", d)
		time.Sleep(d)
		logger.Info("voting ...")
		_, err = postVote(caseID, maxKey, content)
		if err != nil {
			logger.Fatal("failed to vote:", err.Error())
		}
		d = randomDelay()
		logger.Infof("vote succeeded. sleeping for %v to next round...", d)
		split()
		time.Sleep(d)
		// 成功投出一案, 重置连续跳过计数
		skipCount = 0
	}
}

func split() {
	logger.Info(strings.Repeat("-", 50))
}

func buildVoteItem(info caseInfo) map[int]string {
	voteItem := map[int]string{}
	for _, item := range info.VoteItems {
		voteItem[item.Vote] = item.VoteText
	}
	return voteItem
}
