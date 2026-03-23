<?php

namespace App\Repository;

use App\Entity\Product;

class ProductRepository
{
    /** @var array<int, Product> */
    private array $products = [];

    public function find(int $id): ?Product
    {
        return $this->products[$id] ?? null;
    }

    /**
     * @return array<int, Product>
     */
    public function findAll(): array
    {
        return $this->products;
    }

    /**
     * @return array<string, Product>
     */
    public function findByName(string $name): array
    {
        return array_filter($this->products, fn(Product $p) => $p->getName() === $name);
    }

    public function save(Product $product): void
    {
        $this->products[] = $product;
    }
}
