# bilibili-judgement

B 站风纪委员自动投票工具（Go）：自动领取众裁案件，跟随多数意见投票挂机。基于 RimuruChan/bilibili-Judgement 修复维护，适配当前 B 站接口。

## 用法

```bash
go build -o bilibiliJudgement.exe .
./bilibiliJudgement.exe <bili_jct> <SESSDATA>
```

凭证从浏览器登录 B 站后的 Cookie 中获取（`bili_jct` 与 `SESSDATA`）。

Windows 双击挂机可参考 `start.bat.example`（复制为 `start.bat` 填入凭证，已被 gitignore 排除）。
