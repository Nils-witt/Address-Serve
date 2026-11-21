import {
  Button,
  Link,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import { useTranslation } from 'react-i18next';
import RowActions from '../../components/RowActions';
import type { HouseNumber } from '../../api/types';
import { formatCoordinates, formatHouseNumber, osmUrl } from '../../lib/format';
import './HouseNumbersTable.scss';

export default function HouseNumbersTable({
  houseNumbers,
  loading,
  onCreate,
  onEdit,
  onDelete,
}: {
  /** Already in street order. */
  houseNumbers: HouseNumber[];
  loading: boolean;
  onCreate: () => void;
  onEdit: (h: HouseNumber) => void;
  onDelete: (h: HouseNumber) => void;
}) {
  const { t } = useTranslation();
  return (
    <Paper className="house-numbers-table">
      <div className="house-numbers-table__header">
        <Typography variant="h6" component="h2">
          {t('houseNumbers.title', { count: houseNumbers.length })}
        </Typography>
        <Button variant="contained" startIcon={<AddIcon />} onClick={onCreate}>
          {t('houseNumbers.newHouseNumber')}
        </Button>
      </div>
      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('fields.number')}</TableCell>
              <TableCell>{t('fields.postcode')}</TableCell>
              <TableCell>{t('fields.coordinates')}</TableCell>
              <TableCell align="right">{t('common.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {houseNumbers.map((h) => (
              <TableRow key={h.id} hover>
                <TableCell>{formatHouseNumber(h)}</TableCell>
                <TableCell>{h.postcode}</TableCell>
                <TableCell className="house-numbers-table__coordinates">
                  <Link
                    href={osmUrl(h.latitude, h.longitude)}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {formatCoordinates(h.latitude, h.longitude)}
                  </Link>
                </TableCell>
                <TableCell align="right">
                  <RowActions
                    collectorId={h.collectorId}
                    onEdit={() => onEdit(h)}
                    onDelete={() => onDelete(h)}
                  />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      {!loading && houseNumbers.length === 0 && (
        <Typography variant="body2" color="text.secondary" className="house-numbers-table__empty">
          {t('houseNumbers.empty')}
        </Typography>
      )}
    </Paper>
  );
}
