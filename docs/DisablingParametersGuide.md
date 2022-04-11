# Disabling Query Parameters Guide

## Background

The indexer has various endpoints that you (the user) can query.  However, some of these endpoints contain parameters that cause significant query performance degradation, ultimately leading to a scenario where your Indexer instance can be significantly slowed.

In order to alleviate this, the Indexer has the ability to enable or disable query parameters from being supplied. Query parameters that are configured to be disabled stop the query from being run and instead return an error message to the user.

### Types of Parameters

There are two types of parameters: required parameters and optional parameters.  Required parameters are parameters that MUST be supplied to the endpoint.  Optional parameters are parameters that CAN be supplied but don't have to.  

You can see whether parameters are required or optional at the [Algorand Developer Docs](https://developer.algorand.org/docs/rest-apis/indexer/).

__Disabling a required parameter will result in the endpoint being disabled.__

__Disabling an optional parameter will result in the query failing ONLY if the optional parameter is provided.__

### Configuration Schema

The configuration file that is used to enable/disable parameters is a YAML file that has the schema shown below:

```
/v2/accounts:
    optional:
        - currency-greater-than: disabled
        - currency-less-than: disabled
/v2/assets/{asset-id}/transactions:
    optional:
        - note-prefix: disabled
        - tx-type: disabled
        - sig-type: disabled
        - before-time: disabled
        - after-time: disabled
        - currency-greater-than: disabled
        - currency-less-than: disabled
        - address-role: disabled
        - exclude-close-to: disabled
        - rekey-to: disabled
    required:
        - asset-id: disabled
```

The first "level" is a key-value pair where the key is the REST path to the endpoint and the value is made up of up to two sub-dictionaries.  The two sub-directories have a key of either `required` or `optional`, representing the parameters that are required or optional for that endpoint.  Each of those parameters can have a string value of `enabled` or `disabled` representing their current state.

As a concrete example: in the above snippet the endpoint `/v2/accounts` has two optional parameters that are disabled: `currency-greater-than` and `currency-less-than`.  Querying that endpoint and providing either of those two parameters would result in an error being returned.

### Error Return Value

If you query an endpoint with a required parameter you will receive a `400` response with a json message explaining the error.

## Runbook

Below is a list of common scenarios that one might run into when trying to enable/disable configurations.  Each section describes recommended ways of achieving success in that scenario.

### How do I see and use the recommended Indexer configuration?

When running the Indexer daemon, one might wish to use the recommended parameter configuration.  To do that, first issue the following command to save the recommended configuration to a file:

```
~$ algorand-indexer api-config --recommended > myconfig.yaml
```

Note that one can not provide the `--api-config-file` flag and the `--recommended` flag at the same time.

Then inspect the configuration file and modify to suite your needs.  Finally, supply the config file to the daemon:

```
~$ algorand-indexer daemon --api-config-file myconfig.yaml
```


### How do I see what is disabled?

By default, the Algorand Indexer will not disable any parameters.  To see which parameters are disabled by a config file, issue the command:

```
~$ algorand-indexer api-config --api-config-file PATH_TO_FILE
```
where `PATH_TO_FILE` is the path to the configuration file. Note, this will output ONLY the disabled parameters in a YAML configuration.  To view all parameters (both enabled and disabled) issue:

```
~$ algorand-indexer api-config --all --api-config-file PATH_TO_FILE
```

The output from the `api-config` command is a valid YAML configuration file.

### How do I supply my own configuration?

Often it is necessary to change what the Indexer disables and/or enables.  To do this first:

1) Determine what endpoint you wish to disable and find the endpoint path.  One can do this by looking at the [Algorand Developer Docs](https://developer.algorand.org/docs/rest-apis/indexer/).
2) Determine which parameters you wish to disable and whether they are optional or required.
3) Build up the configuration file with the schema described above.
   1) __NOTE:__ The `api-config --recommended` command outputs a valid YAML configuration file that can be used as a starting point.
4) Validate the configuration file you supplied by issuing the command below.  If the file is valid, then the output will show the original YAML file and the program will return zero.  Otherwise, a list of errors will be printed and one will be returned.
```
~$ algorand-indexer api-config --api-config-file PATH_TO_FILE
```
5) Once your file has been validated, supply it when running the Indexer daemon:
```
~$ algorand-indexer daemon --api-config-file PATH_TO_FILE ...
```