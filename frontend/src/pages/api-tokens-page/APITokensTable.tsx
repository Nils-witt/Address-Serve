import { useState } from 'react';
import {
  Button,
  Chip,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import { useTranslation } from 'react-i18next';
import type { APIToken } from '../../api/types';
import { formatDateTime } from '../../lib/format';
import './APITokensTable.scss';

export default function APITokensTable({
  tokens,
  loading,
  onCreate,
  onRevoke,
}: {
  tokens: APIToken[];
  loading: boolean;
  onCreate: () => void;
  onRevoke: (token: APIToken) => void;
}) {
  const { t, i18n } = useTranslation();
  // When the page was opened; good enough to flag expired tokens.
  const [now] = useState(Date.now);
  const date = (iso: string) => formatDateTime(iso, i18n.language);

  return (
    <Paper className="api-tokens-table">
      <div className="api-tokens-table__header">
        <div>
          <Typography variant="h6" component="h1">
            {t('apiTokens.title')}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {t('apiTokens.intro')}
          </Typography>
        </div>
        <Button variant="contained" startIcon={<AddIcon />} onClick={onCreate}>
          {t('apiTokens.newToken')}
        </Button>
      </div>
      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('apiTokens.name')}</TableCell>
              <TableCell>{t('apiTokens.token')}</TableCell>
              <TableCell>{t('apiTokens.created')}</TableCell>
              <TableCell>{t('apiTokens.expires')}</TableCell>
              <TableCell>{t('apiTokens.lastUsed')}</TableCell>
              <TableCell align="right">{t('common.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {tokens.map((token) => {
              const expired = token.expiresAt !== null && Date.parse(token.expiresAt) <= now;
              return (
                <TableRow key={token.id} hover>
                  <TableCell>{token.name}</TableCell>
                  <TableCell className="api-tokens-table__prefix">{token.prefix}…</TableCell>
                  <TableCell>{date(token.createdAt)}</TableCell>
                  <TableCell>
                    {token.expiresAt ? date(token.expiresAt) : t('apiTokens.never')}
                    {expired && (
                      <Chip
                        label={t('apiTokens.expired')}
                        size="small"
                        color="error"
                        variant="outlined"
                        className="api-tokens-table__expired"
                      />
                    )}
                  </TableCell>
                  <TableCell>
                    {token.lastUsedAt ? date(token.lastUsedAt) : t('apiTokens.neverUsed')}
                  </TableCell>
                  <TableCell align="right">
                    <Tooltip title={t('apiTokens.revoke')}>
                      <IconButton size="small" color="error" onClick={() => onRevoke(token)}>
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
      {!loading && tokens.length === 0 && (
        <Typography variant="body2" color="text.secondary" className="api-tokens-table__empty">
          {t('apiTokens.empty')}
        </Typography>
      )}
    </Paper>
  );
}
