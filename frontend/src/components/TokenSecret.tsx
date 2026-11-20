import { useState } from 'react';
import {
  Alert,
  Button,
  IconButton,
  InputAdornment,
  Stack,
  TextField,
  Tooltip,
} from '@mui/material';
import CheckIcon from '@mui/icons-material/Check';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useTranslation } from 'react-i18next';
import './TokenSecret.scss';

/** Shows a token secret once, with a warning and a copy button, for the
 * dialog that just created it. */
export default function TokenSecret({
  label,
  secret,
  onClose,
}: {
  label: string;
  secret: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
    } catch {
      setCopyFailed(true);
    }
  };

  return (
    <Stack spacing={3} className="token-secret">
      <Alert severity="warning">{t('apiTokens.copyNow')}</Alert>
      <TextField
        label={label}
        size="small"
        value={secret}
        error={copyFailed}
        helperText={copyFailed ? t('apiTokens.copyFailed') : undefined}
        slotProps={{
          htmlInput: { readOnly: true, className: 'token-secret__value' },
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
      <Stack direction="row" className="token-secret__actions">
        <Button variant="contained" onClick={onClose}>
          {t('apiTokens.done')}
        </Button>
      </Stack>
    </Stack>
  );
}
