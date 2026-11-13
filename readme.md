# bilibili-judgement

B 站风纪委员自动投票工具（Go）：自动领取众裁案件，跟随多数意见投票挂机。基于 RimuruChan/bilibili-Judgement 修复维护，适配当前 B 站接口。扫码登录，凭证保存本地、失效自动弹码重登，免手动维护。

## 用法

```bash
go build -o bilibiliJudgement.exe .
./bilibiliJudgement.exe            # 无参数: 扫码登录, 凭证保存到 cookies.txt
./bilibiliJudgement.exe <bili_jct> <SESSDATA>   # 或手动传凭证
```

Windows 双击挂机可参考 `start.bat.example`（复制为 `start.bat` 使用）。
