# lTop

CLI tool for log files monitoring

## Introduction

`lTop` is using gorilla encoding and compression to keep metrics about log files in memory. More information about the implementation could be found [here](https://blog.acolyer.org/2016/05/03/gorilla-a-fast-scalable-in-memory-time-series-database/). Further we have used prometheus implemnation for gorilla encoding. The is copies in `chunkenc` directory. Further we borrowed some ideas and code form prometheus client and `tsdb` implementation.

## Future Improvements

Currently `lTop` has only a counter implementation. Adding other types of metrics (like gauge, histogram) will be useful for some particular stats like bytes sent of access log.

Further the design of `lTop` is fixable to add more filter to different types of log files. The only supported type now is `http-access-log`

Adding support for more query functions will be useful to estimate function over `Series` or `Matrix`(set of Series)

## Build and Usage

To build `lTop`:

```bash
make
```

To get help for using the `lTop`

```bash
ltop --help
```

The supported filter for now are:

* `http-access-log`

Example:

```bash
./ltop -l access.log -f http-access-log -c 5 -e 10
```
