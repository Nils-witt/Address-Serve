import { type SubmitEvent, useState } from 'react';
import {
  Alert,
  Button,
  IconButton,
  InputAdornment,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
} from '@mui/material';
import CheckIcon from '@mui/icons-material/Check';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useTranslation } from 'react-i18next';
import type { APITokenInput, CreatedAPIToken } from '../../api/types';
import ErrorBanner from '../../components/ErrorBanner';
import Modal from '../../components/Modal';
import { errorMessage } from '../../lib/errors';
import './APITokenDialog.scss';

/** Expiry choices in days; 0 means the token never expires. */
const EXPIRY_DAYS = [30, 90, 365, 0] as const;
const DEFAULT_EXPIRY_DAYS = 90;
const DAY_MS = 24 * 60 * 60 * 1000;

function expiresAt(days: number): string | null {
  return days ? new Date(Date.now() + days * DAY_MS).toISOString() : null;
}

function TokenForm({
  onClose,
  onSubmit,
}: {
  onClose: () => void;
  onSubmit: (input: APITokenInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [expiryDays, setExpiryDays] = useState<number>(DEFAULT_EXPIRY_DAYS);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await onSubmit({ name: name.trim(), expiresAt: expiresAt(expiryDays) });
    } catch (err) {
      setError(errorMessage(err));
      setSubmitting(false);
    }
  };

  return (
    <Stack component="form" spacing={3} className="api-token-dialog__body" onSubmit={handleSubmit}>
      <ErrorBanner message={error} />
      <TextField
        label={t('apiTokens.name')}
        helperText={t('apiTokens.nameHelp')}
        size="small"
        required
        autoFocus
        slotProps={{ htmlInput: { maxLength: 100 } }}
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      <TextField
        select
        label={t('apiTokens.expiry')}
        size="small"
        value={expiryDays}
        onChange={(e) => setExpiryDays(Number(e.target.value))}
      >
        {EXPIRY_DAYS.map((days) => (
          <MenuItem key={days} value={days}>
            {days ? t('apiTokens.expiryDays', { count: days }) : t('apiTokens.never')}
          </MenuItem>
        ))}
      </TextField>
      <Stack direction="row" spacing={1} className="api-token-dialog__actions">
        <Button onClick={onClose} disabled={submitting}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" variant="contained" disabled={submitting}>
          {t('common.create')}
        </Button>
      </Stack>
    </Stack>
  );
}

function CreatedToken({ token, onClose }: { token: CreatedAPIToken; onClose: () => void }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(token.token);
      setCopied(true);
    } catch {
      setCopyFailed(true);
    }
  };

  return (
    <Stack spacing={3} className="api-token-dialog__body">
      <Alert severity="warning">{t('apiTokens.copyNow')}</Alert>
      <TextField
        label={token.name}
        size="small"
        value={token.token}
        error={copyFailed}
        helperText={copyFailed ? t('apiTokens.copyFailed') : undefined}
        slotProps={{
          htmlInput: { readOnly: true, className: 'api-token-dialog__secret' },
          input: {
            endAdornment: (
              <InputAdornment position="end">
                <Tooltip title={copied ? t('apiTokens.copied') : t('apiTokens.copy')}>
                  <IconButton
                    aria-label={t('apiTokens.copy')}
                    onClick={() => void copy()}
                    edge="end"
                  >
                    {copied ? <CheckIcon /> : <ContentCopyIcon />}
                  </IconButton>
                </Tooltip>
              </InputAdornment>
            ),
          },
        }}
        onFocus={(e) => e.target.select()}
      />
      <Stack direction="row" className="api-token-dialog__actions">
        <Button variant="contained" onClick={onClose}>
          {t('apiTokens.done')}
        </Button>
      </Stack>
    </Stack>
  );
}

/** Creates an API token, then shows its secret once. Give it a new `key` for
 * each opening, so it starts over at the form and the last secret is dropped,
 * while a closing dialog keeps showing what it showed. */
export default function APITokenDialog({
  open,
  onClose,
  onCreate,
}: {
  open: boolean;
  onClose: () => void;
  onCreate: (input: APITokenInput) => Promise<CreatedAPIToken>;
}) {
  const { t } = useTranslation();
  const [created, setCreated] = useState<CreatedAPIToken | null>(null);

  return (
    <Modal
      open={open}
      title={created ? t('apiTokens.createdTitle') : t('apiTokens.newTitle')}
      onClose={onClose}
    >
      {created ? (
        <CreatedToken token={created} onClose={onClose} />
      ) : (
        <TokenForm
          onClose={onClose}
          onSubmit={async (input) => setCreated(await onCreate(input))}
        />
      )}
    </Modal>
  );
}
