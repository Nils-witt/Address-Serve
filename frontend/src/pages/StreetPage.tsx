import { useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Breadcrumbs, Link as MuiLink, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type {
  GeoObject,
  GeoObjectInput,
  HouseNumber,
  HouseNumberInput,
  StreetInput,
} from '../api/types';
import ErrorBanner from '../components/ErrorBanner';
import PageNotice from '../components/PageNotice';
import RouteFallback from '../components/RouteFallback';
import StreetDialog from '../features/streets/StreetDialog';
import { useApi } from '../hooks/useApi';
import { useConfirm } from '../hooks/useConfirm';
import { useGeoObjects } from '../hooks/useGeoObjects';
import { useHouseNumbers } from '../hooks/useHouseNumbers';
import { useInvalidate } from '../hooks/useInvalidate';
import { useStreet } from '../hooks/useStreets';
import { errorMessage } from '../lib/errors';
import { compareHouseNumbers, formatHouseNumber } from '../lib/format';
import { ROUTES } from '../routes';
import GeoObjectDialog from '../features/geo-objects/GeoObjectDialog';
import HouseNumberDialog from '../features/house-numbers/HouseNumberDialog';
import GeoObjectsTable from './street-page/GeoObjectsTable';
import HouseNumbersTable from './street-page/HouseNumbersTable';
import StreetDetails from './street-page/StreetDetails';
import './StreetPage.scss';

/** The most common postcode on the street, to prefill a new house number. */
function commonPostcode(houseNumbers: HouseNumber[]): string {
  const counts = new Map<string, number>();
  for (const h of houseNumbers) counts.set(h.postcode, (counts.get(h.postcode) ?? 0) + 1);
  let best = '';
  let bestCount = 0;
  for (const [postcode, count] of counts) {
    if (count > bestCount) [best, bestCount] = [postcode, count];
  }
  return best;
}

export default function StreetPage() {
  const { t } = useTranslation();
  const { streetId = '' } = useParams<{ streetId: string }>();
  const api = useApi();
  const confirm = useConfirm();
  const invalidate = useInvalidate();
  const navigate = useNavigate();

  const street = useStreet(streetId);
  const houseNumbers = useHouseNumbers(streetId);
  const sortedHouseNumbers = useMemo(
    () => [...houseNumbers.data].sort(compareHouseNumbers),
    [houseNumbers.data],
  );
  const houseNumbersById = useMemo(
    () => new Map(houseNumbers.data.map((h) => [h.id, h])),
    [houseNumbers.data],
  );
  const geoObjectFilter = useMemo(() => ({ streetId }), [streetId]);
  const geoObjects = useGeoObjects(geoObjectFilter);
  const sortedGeoObjects = useMemo(
    () =>
      [...geoObjects.data].sort(
        (a, b) => a.group.localeCompare(b.group) || a.name.localeCompare(b.name),
      ),
    [geoObjects.data],
  );

  const [streetDialogOpen, setStreetDialogOpen] = useState(false);
  // The edited house number stays set while the dialog closes, so its form
  // doesn't flip to "new house number" during the exit animation.
  const [houseNumberDialogOpen, setHouseNumberDialogOpen] = useState(false);
  const [editingHouseNumber, setEditingHouseNumber] = useState<HouseNumber | null>(null);
  const [geoObjectDialogOpen, setGeoObjectDialogOpen] = useState(false);
  const [editingGeoObject, setEditingGeoObject] = useState<GeoObject | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (street.loading) return <RouteFallback />;
  if (!street.data) {
    return <PageNotice message={street.error ?? t('street.notFound')} />;
  }
  const current = street.data;

  const onUpdateStreet = async (input: StreetInput) => {
    await api.updateStreet(current.id, input);
    await invalidate('streets');
  };

  const onDeleteStreet = async () => {
    const ok = await confirm({
      title: t('streets.deleteTitle'),
      message: t('streets.deleteMessage', { name: current.name, city: current.city }),
      confirmLabel: t('common.delete'),
      destructive: true,
    });
    if (!ok) return;
    setError(null);
    try {
      await api.deleteStreet(current.id);
      await navigate(ROUTES.streets, { replace: true });
      await Promise.all([invalidate('streets'), invalidate('houseNumbers')]);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const openHouseNumberDialog = (h: HouseNumber | null) => {
    setEditingHouseNumber(h);
    setHouseNumberDialogOpen(true);
  };

  const onSubmitHouseNumber = async (input: HouseNumberInput) => {
    if (editingHouseNumber) {
      await api.updateHouseNumber(editingHouseNumber.id, input);
    } else {
      await api.createHouseNumber(input);
    }
    await invalidate('houseNumbers');
  };

  const onDeleteHouseNumber = async (h: HouseNumber) => {
    const ok = await confirm({
      title: t('houseNumbers.deleteTitle'),
      message: t('houseNumbers.deleteMessage', {
        number: formatHouseNumber(h),
        street: current.name,
      }),
      confirmLabel: t('common.delete'),
      destructive: true,
    });
    if (!ok) return;
    setError(null);
    try {
      await api.deleteHouseNumber(h.id);
      await invalidate('houseNumbers');
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const openGeoObjectDialog = (g: GeoObject | null) => {
    setEditingGeoObject(g);
    setGeoObjectDialogOpen(true);
  };

  const onSubmitGeoObject = async (input: GeoObjectInput) => {
    if (editingGeoObject) {
      await api.updateGeoObject(editingGeoObject.id, input);
    } else {
      await api.createGeoObject(input);
    }
    await invalidate('geoObjects');
  };

  const onDeleteGeoObject = async (g: GeoObject) => {
    const ok = await confirm({
      title: t('geoObjects.deleteTitle'),
      message: t('geoObjects.deleteMessage', { name: g.name, group: g.group }),
      confirmLabel: t('common.delete'),
      destructive: true,
    });
    if (!ok) return;
    setError(null);
    try {
      await api.deleteGeoObject(g.id);
      await invalidate('geoObjects');
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <>
      <Breadcrumbs className="street-page__breadcrumbs">
        <MuiLink component={Link} to={ROUTES.streets} underline="hover" color="inherit">
          {t('nav.streets')}
        </MuiLink>
        <MuiLink
          component={Link}
          to={`${ROUTES.streets}?${new URLSearchParams({ city: current.city })}`}
          underline="hover"
          color="inherit"
        >
          {current.city}
        </MuiLink>
        <Typography color="text.primary">{current.name}</Typography>
      </Breadcrumbs>
      <ErrorBanner message={error ?? houseNumbers.error ?? geoObjects.error} />
      <StreetDetails
        street={current}
        onEdit={() => setStreetDialogOpen(true)}
        onDelete={() => void onDeleteStreet()}
      />
      <HouseNumbersTable
        houseNumbers={sortedHouseNumbers}
        loading={houseNumbers.loading}
        onCreate={() => openHouseNumberDialog(null)}
        onEdit={openHouseNumberDialog}
        onDelete={(h) => void onDeleteHouseNumber(h)}
      />
      <GeoObjectsTable
        geoObjects={sortedGeoObjects}
        houseNumbersById={houseNumbersById}
        loading={geoObjects.loading}
        onCreate={() => openGeoObjectDialog(null)}
        onEdit={openGeoObjectDialog}
        onDelete={(g) => void onDeleteGeoObject(g)}
      />
      <StreetDialog
        open={streetDialogOpen}
        street={current}
        onClose={() => setStreetDialogOpen(false)}
        onSubmit={onUpdateStreet}
      />
      <HouseNumberDialog
        open={houseNumberDialogOpen}
        streetId={current.id}
        houseNumber={editingHouseNumber}
        defaultPostcode={commonPostcode(houseNumbers.data)}
        onClose={() => setHouseNumberDialogOpen(false)}
        onSubmit={onSubmitHouseNumber}
      />
      <GeoObjectDialog
        open={geoObjectDialogOpen}
        geoObject={editingGeoObject}
        street={current}
        fixedStreet
        onClose={() => setGeoObjectDialogOpen(false)}
        onSubmit={onSubmitGeoObject}
      />
    </>
  );
}
