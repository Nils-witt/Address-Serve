import { useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import {
  IconButton,
  Link,
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
import type { HouseNumber, Street } from '../../api/types';
import { formatCoordinates, formatHouseNumber, osmUrl } from '../../lib/format';
import { ROUTES } from '../../routes';
import './HouseNumbersListTable.scss';

const ROWS_PER_PAGE_OPTIONS = [25, 50, 100];

export interface HouseNumberRow {
  houseNumber: HouseNumber;
  /** Undefined until the streets have loaded. */
  street: Street | undefined;
}

export default function HouseNumbersListTable({
  rows,
  loading,
  onEdit,
  onDelete,
}: {
  /** Already in display order. */
  rows: HouseNumberRow[];
  loading: boolean;
  onEdit: (h: HouseNumber) => void;
  onDelete: (row: HouseNumberRow) => void;
}) {
  const { t } = useTranslation();
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(ROWS_PER_PAGE_OPTIONS[1]);

  // A narrower filter can leave the current page past the end.
  const lastPage = Math.max(0, Math.ceil(rows.length / rowsPerPage) - 1);
  const currentPage = Math.min(page, lastPage);
  const visible = rows.slice(currentPage * rowsPerPage, (currentPage + 1) * rowsPerPage);

  return (
    <Paper className="house-numbers-list-table">
      <TableContainer>
        <Table stickyHeader size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('fields.streetName')}</TableCell>
              <TableCell>{t('fields.number')}</TableCell>
              <TableCell>{t('fields.postcode')}</TableCell>
              <TableCell>{t('fields.district')}</TableCell>
              <TableCell>{t('fields.city')}</TableCell>
              <TableCell>{t('fields.coordinates')}</TableCell>
              <TableCell align="right">{t('common.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {visible.map(({ houseNumber: h, street }) => (
              <TableRow key={h.id} hover>
                <TableCell>
                  <Link component={RouterLink} to={ROUTES.street(h.streetId)} underline="hover">
                    {street?.name ?? h.streetId}
                  </Link>
                </TableCell>
                <TableCell>{formatHouseNumber(h)}</TableCell>
                <TableCell>{h.postcode}</TableCell>
                <TableCell>{street?.district}</TableCell>
                <TableCell>{street?.city}</TableCell>
                <TableCell className="house-numbers-list-table__coordinates">
                  <Link
                    href={osmUrl(h.latitude, h.longitude)}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {formatCoordinates(h.latitude, h.longitude)}
                  </Link>
                </TableCell>
                <TableCell align="right">
                  <Tooltip title={t('common.edit')}>
                    <IconButton size="small" onClick={() => onEdit(h)}>
                      <EditIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title={t('common.delete')}>
                    <IconButton
                      size="small"
                      color="error"
                      onClick={() => onDelete({ houseNumber: h, street })}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      {!loading && rows.length === 0 && (
        <Typography
          variant="body2"
          color="text.secondary"
          className="house-numbers-list-table__empty"
        >
          {t('houseNumbers.emptyList')}
        </Typography>
      )}
      <TablePagination
        component="div"
        count={rows.length}
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
