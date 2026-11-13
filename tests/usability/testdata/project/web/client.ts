export interface Account {
  id: string;
  displayName: string;
  email: string;
}

export interface AccountGateway {
  getAccount(id: string, signal?: AbortSignal): Promise<Account>;
}

export class AccountClient {
  constructor(
    private readonly baseURL: string,
    private readonly token: () => string,
    private readonly fetcher: typeof fetch = fetch,
  ) {}

  async getAccount(id: string, signal?: AbortSignal): Promise<Account> {
    const response = await this.fetcher(
      `${this.baseURL}/accounts/${encodeURIComponent(id)}`,
      {
        headers: { Authorization: `Bearer ${this.token()}` },
        signal,
      },
    );

    if (!response.ok) {
      throw new Error(`Account request failed: ${response.status}`);
    }

    return (await response.json()) as Account;
  }
}
