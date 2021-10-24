# MtBot

![Go](https://github.com/z0rr0/mtbot/workflows/Go/badge.svg)
![Version](https://img.shields.io/github/tag/z0rr0/mtbot.svg)
![License](https://img.shields.io/github/license/z0rr0/mtbot.svg)

MyTeam event notification bot.

## Build

```shell
go install .
```

### Test

```
go test -race -cover -v ./...
```

### Run

Config example file is config.toml

```shell
./mtbot -config $COFIG_FILE
```

## License

This source code is governed by a MIT license that can be found
in the [LICENSE](https://github.com/z0rr0/mtbot/blob/main/LICENSE) file.