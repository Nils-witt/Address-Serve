import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { APIToken, APITokenInput } from '../api/types';
import ErrorBanner from '../components/ErrorBanner';
import APITokenDialog from '../features/api-tokens/APITokenDialog';
import { useApi } from '../hooks/useApi';
import { useAPITokens } from '../hooks/useAPITokens';
import { useConfirm } from '../hooks/useConfirm';
import { useInvalidate } from '../hooks/useInvalidate';
import { errorMessage } from '../lib/errors';
import APITokensTable from './api-tokens-page/APITokensTable';

export default function APITokensPage() {
  const { t } = useTranslation();
  const api = useApi();
  const confirm = useConfirm();
  const invalidate = useInvalidate();
  const { data: tokens, error: loadError, loading } = useAPITokens();

  const [dialogOpen, setDialogOpen] = useState(false);
  // Bumped on each opening, so the dialog remounts with an empty form.
  const [dialogKey, setDialogKey] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const openDialog = () => {
    setDialogKey((k) => k + 1);
    setDialogOpen(true);
  };

  const onCreate = async (input: APITokenInput) => {
    const created = await api.createAPIToken(input);
    await invalidate('apiTokens');
    return created;
  };

  const onRevoke = async (token: APIToken) => {
    const ok = await confirm({
      title: t('apiTokens.revokeTitle'),
      message: t('apiTokens.revokeMessage', { name: token.name }),
      confirmLabel: t('apiTokens.revoke'),
      destructive: true,
    });
    if (!ok) return;
    setError(null);
    try {
      await api.deleteAPIToken(token.id);
      await invalidate('apiTokens');
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <>
      <ErrorBanner message={error ?? loadError} />
      <APITokensTable
        tokens={tokens}
        loading={loading}
        onCreate={openDialog}
        onRevoke={(token) => void onRevoke(token)}
      />
      <APITokenDialog
        key={dialogKey}
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        onCreate={onCreate}
      />
    </>
  );
}
