VEGAMM
======

A simple market maker bot to use with the [vega protocol](https://github.com/vegaprotocol/vega).

This software requires the vega wallet to run as a services separately. You will need go installed and configured properly on your environment.

## Requirements:

### Vega toolchain

First install the vega toolchain using:
```
go install code.vegaprotocol.io/vega@latest
```

### Setting up the Vega wallet

Then initialize a wallet:
```
vega wallet init --home=YOUR_WALLET_HOME
```

Then initialize the support for the token api (a passphrase will be required):
```
vega wallet create --home=YOUR_WALLET_HOME --wallet=YOUR_TEST_WALLET
```
The command line should output a public key and mnemonic words. Copy the public key it will be required later.

Then initialize the token api:
```
vega wallet api-token init --home=YOUR_WALLET_HOME
```

Then generate a token for the wallet previously created:
```
vega wallet api-token generate --wallet-name=YOUR_WALLET_HOME --home=YOUR_TEST_WALLET
```
This command should output a token, be sure to save it as well.

Finally start the veg wallet service (with support for tokens):
```
vega wallet service run --no-version-check --network=fairground --load-tokens
```

## Building vegamm

The binary can be installed with the following command at the root of the repository:
```
go install
```

## Use vegamm

vegamm requirement some configuration to be started. The configuration is taken ever from the command line through flags or from the environement. For example, the market to trade on, can be specified through the command line with the following flag `-vega-market` or through the environment with the following variable if specified `VEGAMM_VEGA_MARKET`. The same pattern (note the prefix on the environement variable) is applicable for every arguments.

For example to start the bot against the `UNIDAI.MF21` market on fairground, the following command would be needed:
```
vegamm -wallet-token="THE_TOKEN" -wallet-pubkey="YOUR_PUBLIC_KEY" -vega-market="325dfa07e1be5192376616241d23b4d71740fe712e298130bfd35d27738f1ce4" -binance-market="UNIUSDT"
```

_*Note*_: For the bots to be able to trade, you'll have to deposit funds on the general account of your public key.

For more information on the available flags you can run:
```
vegamm -help
```

## LICENCE

This software is provided under the MIT license.
