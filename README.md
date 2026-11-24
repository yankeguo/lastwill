# lastwill

A standalone system for disclosing my last will automatically

View https://yankeguo.github.io/lastwill

## Encrypted data

The [`@yankeguo/lastwill-data`](https://www.npmjs.com/package/@yankeguo/lastwill-data) npm package includes both the encrypted payload and the decryption CLI. No separate ciphertext download, beacon value, or repository checkout is needed.

After the disclosure page shows a decryption key, run:

```sh
npx @yankeguo/lastwill-data
npx @yankeguo/lastwill-data --format html
```

Paste the key at the hidden `Decryption password:` prompt. The CLI prints decrypted Markdown by default, or HTML with `--format html`, to standard output. Use a private terminal and protect any saved output as sensitive.

For repeated use, install the CLI globally with `npm install --global @yankeguo/lastwill-data`, then run `yankeguo-lastwill-data`.

## Credits

Y.-K. Guo, MIT License
