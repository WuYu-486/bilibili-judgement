package main

import (
	"github.com/withmandala/go-log"
	"os"
	"strings"
	"time"
)

// Version of Program
const Version = "1.0"

var sessdata string
var csrf string

var logger = log.New(os.Stderr)

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
	// 跟投所需的最少意见数; 原版为 10, 凌晨新案意见稀少根本凑不齐, 降为 3
	const minVotes = 3
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
					logger.Infof("Votes amount less than %d (current: %d), retry fetching after 10 seconds...", minVotes, len(opinion.List))
				}
				time.Sleep(10 * time.Second)
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
		logger.Info("VotedKey:		", maxKey)
		logger.Info("VotedTimes:		", maxValue)
		logger.Info("TotalVotedTimes:	", opinion.Total)
		logger.Info("VoteText:		", voteItem[maxKey])
		split()
		logger.Info("claiming case...")
		_, err = postVote(caseID, 0)
		if err != nil {
			logger.Fatal("failed to claim case:", err.Error())
		}
		logger.Info("claim succeeded. sleeping for 15s...")
		time.Sleep(15 * time.Second)
		logger.Info("voting ...")
		_, err = postVote(caseID, maxKey)
		if err != nil {
			logger.Fatal("failed to vote:", err.Error())
		}
		logger.Info("vote succeeded. sleeping for 15s to next round...")
		split()
		time.Sleep(15 * time.Second)
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
