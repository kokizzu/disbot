# EgaoShark Media Archiver

Small Discord bot utility that archives image attachments from one authorized
server channel. It uses a dedicated bot account, never a personal user token.

## Configuration

Copy `.env.example` to `.env`, set `DISCORD_BOT_TOKEN`, and keep `.env` local.
The default source and destination are:

- server: `EgaoShark Waifu Culture`
- channel: `nsfw_perfect`
- directory: `/home/kyz/Pictures/EgaoSharkWaifuCulture`

The bot needs only `View Channel` and `Read Message History` in the source
channel. Use `make bot-info` to print the least-privilege installation URL, or
`make open-install` to open it in the local browser.

## Workflow

```text
make verify
make bot-info
make configure
make open-install
make plan
make download
```

`make plan` scans the visible channel history and saves an ignored,
permission-restricted `.disbot-plan.json` checkpoint. `make download` consumes
that exact checkpoint, skips matching existing files, and refuses to overwrite
a size-mismatched file.

The application and bot icons are generated from `assets/icon.png` by
`make configure` along with the application description and default minimal
installation permissions.
