import { AccountClient } from "./client";

export async function accountClientRejectsFailedResponses(): Promise<void> {
  const client = new AccountClient(
    "https://api.example.test",
    () => "test-token",
    async () => new Response("", { status: 503 }),
  );

  await client.getAccount("acct-7").then(
    () => {
      throw new Error("expected a failed HTTP response to reject");
    },
    (error: unknown) => {
      if (!(error instanceof Error) || !error.message.includes("503")) {
        throw error;
      }
    },
  );
}
