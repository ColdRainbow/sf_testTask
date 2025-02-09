# Staking Facilities: Test Task

This repo is a simple RESTful Ethereum Validator API. It interacts with Ethereum data to provide information about block rewards and validator sync committee duties per slot.

## Installation and running

To use the script, either clone the folder via HTTPS or download the ZIP folder.
- HTTPS: `git clone https://github.com/ColdRainbow/sf_testTask.git`
- ZIP: "Download ZIP" button

After you've obtained the local repo, go to the folder with the script using the command cd. Then, use these commands to run the code:
```bash
go mod tidy
go run .
```

In a separate terminal window, curl an endpoint (e.g. `curl localhost:8080/blockreward/11009611` — more about endpoints in the next section).

## Endpoints

1. GET /blockreward/{slot}

- Purpose: Retrieves information about the block reward for a given slot.
- Parameters:
	- slot (integer): The slot number in the Ethereum blockchain.
- Response: JSON
	- status: Whether the slot contains a block produced by a MEV relay or a vanilla block (built internally in the validator node).
	- reward: The amount of reward the node operator/validator received for including the block in that slot (in GWEI).

- Example: `curl localhost:8080/blockreward/11009611` retrieves the information about the block reward for the slot 11009611.

2. GET /syncduties/{slot}

- Purpose: Retrieves a list of validators that have sync committee duties for a given slot.
- Parameters:
	- slot (integer): The slot number in the Ethereum blockchain.
- Response:
	- A list of public keys of validators that had sync committee duties for the specified slot.

- Example: `curl localhost:8080/syncduties/11003735` retrieves a list of validators for the slot 11003735