import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { useTranslation } from 'react-i18next';
import type { Street } from '../../api/types';
import { formatCoordinates } from '../../lib/format';
import { ROUTES } from '../../routes';
import './StreetsTable.scss';

const ROWS_PER_PAGE_OPTIONS = [25, 50, 100];

export default function StreetsTable({
  streets,
  loading,
  onEdit,
  onDelete,
}: {
  streets: Street[];
  loading: boolean;
  onEdit: (street: Street) => void;
  onDelete: (street: Street) => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(ROWS_PER_PAGE_OPTIONS[1]);

  // A narrower filter can leave the current page past the end.
  const lastPage = Math.max(0, Math.ceil(streets.length / rowsPerPage) - 1);
  const currentPage = Math.min(page, lastPage);
  const visible = streets.slice(currentPage * rowsPerPage, (currentPage + 1) * rowsPerPage);

  return (
    <Paper className="streets-table">
      <TableContainer>
        <Table stickyHeader size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('fields.streetName')}</TableCell>
              <TableCell>{t('fields.district')}</TableCell>
              <TableCell>{t('fields.city')}</TableCell>
              <TableCell>{t('fields.country')}</TableCell>
              <TableCell>{t('fields.coordinates')}</TableCell>
              <TableCell align="right">{t('common.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {visible.map((s) => (
              <TableRow
                key={s.id}
                hover
                className="streets-table__row"
                onClick={() => void navigate(ROUTES.street(s.id))}
              >
                <TableCell>{s.name}</TableCell>
                <TableCell>{s.district}</TableCell>
                <TableCell>{s.city}</TableCell>
                <TableCell>{s.country}</TableCell>
                <TableCell className="streets-table__coordinates">
                  {formatCoordinates(s.latitude, s.longitude)}
                </TableCell>
                <TableCell align="right" onClick={(e) => e.stopPropagation()}>
                  <Tooltip title={t('common.edit')}>
                    <IconButton size="small" onClick={() => onEdit(s)}>
                      <EditIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title={t('common.delete')}>
                    <IconButton size="small" color="error" onClick={() => onDelete(s)}>
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      {!loading && streets.length === 0 && (
        <Typography variant="body2" color="text.secondary" className="streets-table__empty">
          {t('streets.empty')}
        </Typography>
      )}
      <TablePagination
        component="div"
        count={streets.length}
        page={currentPage}
        rowsPerPage={rowsPerPage}
        rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
        labelRowsPerPage={t('common.rowsPerPage')}
        onPageChange={(_, next) => setPage(next)}
        onRowsPerPageChange={(e) => {
          setRowsPerPage(Number(e.target.value));
          setPage(0);
        }}
      />
    </Paper>
  );
}
