import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { Street, StreetInput } from '../api/types';
import ErrorBanner from '../components/ErrorBanner';
import StreetDialog from '../features/streets/StreetDialog';
import { useApi } from '../hooks/useApi';
import { useConfirm } from '../hooks/useConfirm';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { useInvalidate } from '../hooks/useInvalidate';
import { useStreets } from '../hooks/useStreets';
import { errorMessage } from '../lib/errors';
import StreetsTable from './streets-page/StreetsTable';
import StreetsToolbar from './streets-page/StreetsToolbar';

export default function StreetsPage() {
  const { t } = useTranslation();
  const api = useApi();
  const confirm = useConfirm();
  const invalidate = useInvalidate();

  // The filters live in the URL, so a filtered list can be linked and the
  // back button from a street returns to it.
  const [searchParams, setSearchParams] = useSearchParams();
  const city = searchParams.get('city') ?? '';
  const district = searchParams.get('district') ?? '';
  const [name, setName] = useState(searchParams.get('name') ?? '');
  const debouncedName = useDebouncedValue(name.trim());

  const setFilter = (key: string, value: string, reset: string[] = []) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value) next.set(key, value);
        else next.delete(key);
        for (const k of reset) next.delete(k);
        return next;
      },
      { replace: true },
    );
  };

  const filter = useMemo(
    () => ({ city, district, name: debouncedName }),
    [city, district, debouncedName],
  );
  const { data: streets, error: loadError, loading } = useStreets(filter);

  // The edited street stays set while the dialog closes, so its form doesn't
  // flip to "new street" during the exit animation.
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingStreet, setEditingStreet] = useState<Street | null>(null);
  const [error, setError] = useState<string | null>(null);

  const openDialog = (street: Street | null) => {
    setEditingStreet(street);
    setDialogOpen(true);
  };

  const onSubmit = async (input: StreetInput) => {
    if (editingStreet) {
      await api.updateStreet(editingStreet.id, input);
    } else {
      await api.createStreet(input);
    }
    await invalidate('streets');
  };

  const onDelete = async (street: Street) => {
    const ok = await confirm({
      title: t('streets.deleteTitle'),
      message: t('streets.deleteMessage', { name: street.name, city: street.city }),
      confirmLabel: t('common.delete'),
      destructive: true,
    });
    if (!ok) return;
    setError(null);
    try {
      await api.deleteStreet(street.id);
      await Promise.all([invalidate('streets'), invalidate('houseNumbers')]);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <>
      <StreetsToolbar
        city={city}
        district={district}
        name={name}
        onCityChange={(value) => setFilter('city', value, ['district'])}
        onDistrictChange={(value) => setFilter('district', value)}
        onNameChange={(value) => {
          setName(value);
          setFilter('name', value.trim());
        }}
        onCreate={() => openDialog(null)}
      />
      <ErrorBanner message={error ?? loadError} />
      <StreetsTable
        streets={streets}
        loading={loading}
        onEdit={openDialog}
        onDelete={(s) => void onDelete(s)}
      />
      <StreetDialog
        open={dialogOpen}
        street={editingStreet}
        defaults={{ city, district, country: streets[0]?.country }}
        onClose={() => setDialogOpen(false)}
        onSubmit={onSubmit}
      />
    </>
  );
}
