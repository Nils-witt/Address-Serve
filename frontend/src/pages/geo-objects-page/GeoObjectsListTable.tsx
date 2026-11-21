import { useMemo, useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import {
  Checkbox,
  Link,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import RowActions from '../../components/RowActions';
import type { GeoObject, Street } from '../../api/types';
import { useHouseNumbersById } from '../../hooks/useGeoObjects';
import { formatCoordinates, formatHouseNumber, osmUrl } from '../../lib/format';
import { ROUTES } from '../../routes';
import './GeoObjectsListTable.scss';

const ROWS_PER_PAGE_OPTIONS = [25, 50, 100];

export interface GeoObjectRow {
  geoObject: GeoObject;
  /** Undefined until the streets have loaded. */
  street: Street | undefined;
}

export default function GeoObjectsListTable({
  rows,
  loading,
  selected,
  onSelectedChange,
  onEdit,
  onDelete,
}: {
  /** Already in display order. */
  rows: GeoObjectRow[];
  loading: boolean;
  /** IDs of the selected geo objects, all among `rows`. */
  selected: ReadonlySet<string>;
  onSelectedChange: (selected: Set<string>) => void;
  onEdit: (row: GeoObjectRow) => void;
  onDelete: (row: GeoObjectRow) => void;
}) {
  const { t } = useTranslation();
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(ROWS_PER_PAGE_OPTIONS[1]);

  // A narrower filter can leave the current page past the end.
  const lastPage = Math.max(0, Math.ceil(rows.length / rowsPerPage) - 1);
  const currentPage = Math.min(page, lastPage);
  const visible = rows.slice(currentPage * rowsPerPage, (currentPage + 1) * rowsPerPage);

  // House numbers are looked up per street, so only for the rows on this page.
  const streetIdsKey = [
    ...new Set(visible.filter((r) => r.geoObject.houseNumberId).map((r) => r.geoObject.streetId)),
  ].join(',');
  const streetIds = useMemo(() => (streetIdsKey ? streetIdsKey.split(',') : []), [streetIdsKey]);
  const houseNumbersById = useHouseNumbersById(streetIds);

  // The header checkbox covers every row matching the filter, not just this
  // page, so a whole group can be selected at once. Rows a collector owns
  // can't be deleted here, so they can't be selected.
  const selectable = rows.filter((r) => !r.geoObject.collectorId);
  const allSelected = selectable.length > 0 && selected.size === selectable.length;
  const toggleAll = () =>
    onSelectedChange(allSelected ? new Set() : new Set(selectable.map((r) => r.geoObject.id)));
  const toggle = (id: string) => {
    const next = new Set(selected);
    if (!next.delete(id)) next.add(id);
    onSelectedChange(next);
  };

  return (
    <Paper className="geo-objects-list-table">
      <TableContainer>
        <Table stickyHeader size="small">
          <TableHead>
            <TableRow>
              <TableCell padding="checkbox">
                <Checkbox
                  size="small"
                  checked={allSelected}
                  indeterminate={selected.size > 0 && !allSelected}
                  disabled={selectable.length === 0}
                  onChange={toggleAll}
                  slotProps={{ input: { 'aria-label': t('geoObjects.selectAll') } }}
                />
              </TableCell>
              <TableCell>{t('fields.name')}</TableCell>
              <TableCell>{t('fields.group')}</TableCell>
              <TableCell>{t('fields.streetName')}</TableCell>
              <TableCell>{t('fields.houseNumber')}</TableCell>
              <TableCell>{t('fields.city')}</TableCell>
              <TableCell>{t('fields.coordinates')}</TableCell>
              <TableCell align="right">{t('common.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {visible.map((row) => {
              const { geoObject: g, street } = row;
              const houseNumber = g.houseNumberId
                ? houseNumbersById.get(g.houseNumberId)
                : undefined;
              return (
                <TableRow key={g.id} hover selected={selected.has(g.id)}>
                  <TableCell padding="checkbox">
                    <Checkbox
                      size="small"
                      checked={selected.has(g.id)}
                      disabled={Boolean(g.collectorId)}
                      onChange={() => toggle(g.id)}
                      slotProps={{
                        input: { 'aria-label': t('geoObjects.select', { name: g.name }) },
                      }}
                    />
                  </TableCell>
                  <TableCell>{g.name}</TableCell>
                  <TableCell>{g.group}</TableCell>
                  <TableCell>
                    <Link component={RouterLink} to={ROUTES.street(g.streetId)} underline="hover">
                      {street?.name ?? g.streetId}
                    </Link>
                  </TableCell>
                  <TableCell>{houseNumber ? formatHouseNumber(houseNumber) : ''}</TableCell>
                  <TableCell>{street?.city}</TableCell>
                  <TableCell className="geo-objects-list-table__coordinates">
                    <Link
                      href={osmUrl(g.latitude, g.longitude)}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {formatCoordinates(g.latitude, g.longitude)}
                    </Link>
                  </TableCell>
                  <TableCell align="right">
                    <RowActions
                      collectorId={g.collectorId}
                      onEdit={() => onEdit(row)}
                      onDelete={() => onDelete(row)}
                    />
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
      {!loading && rows.length === 0 && (
        <Typography
          variant="body2"
          color="text.secondary"
          className="geo-objects-list-table__empty"
        >
          {t('geoObjects.emptyList')}
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
