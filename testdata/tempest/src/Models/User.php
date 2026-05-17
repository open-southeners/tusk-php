<?php

declare(strict_types=1);

namespace App\Models;

use Tempest\Database\Model;
use Tempest\Database\IsDatabaseModel;
use App\Attributes\Required;
use App\Attributes\Email;
use App\Attributes\MinLength;

final class User implements Model {
    use IsDatabaseModel;

    public function __construct(
        #[Required]
        #[MinLength(2)]
        public string $name,

        #[Required]
        #[Email]
        public string $email,

        public \DateTimeImmutable $createdAt = new \DateTimeImmutable(),
        public ?\DateTimeImmutable $updatedAt = null,
    ) {}

    public function isValid(): bool
    {
        return $this->name !== '' && str_contains($this->email, '@');
    }

    public function toArray(): array
    {
        return [
            'name'       => $this->name,
            'email'      => $this->email,
            'created_at' => $this->createdAt->format(\DATE_ATOM),
            'updated_at' => $this->updatedAt?->format(\DATE_ATOM),
        ];
    }
}
