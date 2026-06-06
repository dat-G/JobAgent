# Legato Vendor Dependencies

This directory stores offline Python wheels required by Legato's recognition frontend.

Current bundled frontend:

- `markitdown[pdf]==0.1.6`
- `setuptools==82.0.1` for offline editable installation

The wheelhouse is intended to make the Legato package self-contained for the target runtime.

## Offline Install

From `Agent/legato`:

```sh
python3 -m pip install --no-index --find-links vendor/wheelhouse markitdown
```

For a virtual environment:

```sh
python3 -m venv .venv
. .venv/bin/activate
scripts/install_dev_offline.sh
```

## Platform Note

Some downloaded wheels are platform-specific. The current wheelhouse was downloaded on macOS arm64 with Python 3.13.

For Linux, x86_64, or another Python version, regenerate the wheelhouse for that deployment target and keep it under `vendor/wheelhouse`.
